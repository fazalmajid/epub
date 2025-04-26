package main

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"archive/zip"
)

// OPF Container XML structure
type Container struct {
	XMLName  xml.Name `xml:"container"`
	RootFile struct {
		FullPath string `xml:"full-path,attr"`
	} `xml:"rootfiles>rootfile"`
}

// OPF Package structure
type Package struct {
	XMLName  xml.Name `xml:"package"`
	Metadata struct {
		Title       []string `xml:"title"`
		Creator     []string `xml:"creator"`
		Identifier  []string `xml:"identifier"`
		Language    []string `xml:"language"`
		Publisher   []string `xml:"publisher"`
		Description []string `xml:"description"`
		Subject     []string `xml:"subject"`
		Date        []string `xml:"date"`
		Rights      []string `xml:"rights"`
	} `xml:"metadata"`
}

// BookMetadata holds the processed metadata
type BookMetadata struct {
	Title       string   `json:"title"`
	Authors     []string `json:"authors"`
	Identifiers []string `json:"identifiers"`
	Language    string   `json:"language,omitempty"`
	Publisher   string   `json:"publisher,omitempty"`
	Description string   `json:"description,omitempty"`
	Subjects    []string `json:"subjects,omitempty"`
	Date        string   `json:"date,omitempty"`
	Rights      string   `json:"rights,omitempty"`
	Filename    string   `json:"filename"`
	FilePath    string   `json:"filepath"`
	FileSize    int64    `json:"filesize"`
}

// encoding/xml does not support XML 1.1, which is common in ePub metadata
// See https://github.com/golang/go/issues/25755
// XML11To10Reader wraps an io.Reader and converts XML 1.1 declarations to XML 1.0
// This allows processing XML 1.1 documents with Go's encoding/xml package
// which only supports XML 1.0.
func XML11To10Reader(r io.ReadCloser) (io.Reader, error) {
	// Read all content from reader
	content, err := io.ReadAll(r)
	if err != nil {
		// If there's an error reading, return a reader that will return the error
		return nil, err
	}
	r.Close()

	// Check for XML declaration
	if len(content) > 5 && bytes.HasPrefix(content, []byte("<?xml")) {
		// Look for the XML declaration end
		endPos := bytes.Index(content, []byte("?>"))
		if endPos > 0 {
			// Extract just the declaration part
			declaration := content[:endPos+2]

			// Simple replacements for common version patterns
			replacements := []struct {
				old string
				new string
			}{
				{`version="1.1"`, `version="1.0"`},
				{`version='1.1'`, `version='1.0'`},
				{`version = "1.1"`, `version = "1.0"`},
				{`version = '1.1'`, `version = '1.0'`},
				{`version= "1.1"`, `version= "1.0"`},
				{`version= '1.1'`, `version= '1.0'`},
				{`version ="1.1"`, `version ="1.0"`},
				{`version ='1.1'`, `version ='1.0'`},
			}

			// Apply all possible replacements
			declarationStr := string(declaration)
			for _, r := range replacements {
				declarationStr = strings.Replace(declarationStr, r.old, r.new, 1)
			}

			// Combine the modified declaration with the rest of the content
			result := []byte(declarationStr)
			result = append(result, content[endPos+2:]...)

			return bytes.NewReader(result), nil
		}
	}

	// If no XML declaration found or no replacement needed, return original content
	return bytes.NewReader(content), nil
}

// errorReader is a simple reader that just returns an error
type errorReader struct {
	err error
}

func extractMetadata(filePath string) (*BookMetadata, error) {
	// Open the epub file (it's a zip file)
	reader, err := zip.OpenReader(filePath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	// First, find and parse the container.xml file
	var containerFile *zip.File
	for _, file := range reader.File {
		if file.Name == "META-INF/container.xml" {
			containerFile = file
			break
		}
	}

	if containerFile == nil {
		return nil, errors.New("container.xml not found in epub")
	}
	_containerReader, err := containerFile.Open()
	if err != nil {
		return nil, err
	}
	containerReader, err := XML11To10Reader(_containerReader)
	if err != nil {
		return nil, err
	}
	var container Container
	if err := xml.NewDecoder(containerReader).Decode(&container); err != nil {
		return nil, err
	}

	// Now find and parse the OPF file
	opfPath := container.RootFile.FullPath
	if opfPath == "" {
		return nil, errors.New("OPF file path not found in container.xml")
	}

	var opfFile *zip.File
	for _, file := range reader.File {
		if file.Name == opfPath {
			opfFile = file
			break
		}
	}

	if opfFile == nil {
		return nil, errors.New("OPF file not found in epub")
	}

	_opfReader, err := opfFile.Open()
	if err != nil {
		return nil, err
	}
	opfReader, err := XML11To10Reader(_opfReader)
	if err != nil {
		return nil, err
	}
	var pkg Package
	if err := xml.NewDecoder(opfReader).Decode(&pkg); err != nil {
		return nil, err
	}

	// Get file info
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}

	// Extract and organize metadata
	metadata := &BookMetadata{
		Filename: filepath.Base(filePath),
		FilePath: filePath,
		FileSize: fileInfo.Size(),
	}

	if len(pkg.Metadata.Title) > 0 {
		metadata.Title = pkg.Metadata.Title[0]
	}

	for _, creator := range pkg.Metadata.Creator {
		metadata.Authors = append(metadata.Authors, creator)
	}

	for _, identifier := range pkg.Metadata.Identifier {
		metadata.Identifiers = append(metadata.Identifiers, identifier)
	}

	if len(pkg.Metadata.Language) > 0 {
		metadata.Language = pkg.Metadata.Language[0]
	}

	if len(pkg.Metadata.Publisher) > 0 {
		metadata.Publisher = pkg.Metadata.Publisher[0]
	}

	if len(pkg.Metadata.Description) > 0 {
		metadata.Description = pkg.Metadata.Description[0]
	}

	for _, subject := range pkg.Metadata.Subject {
		metadata.Subjects = append(metadata.Subjects, subject)
	}

	if len(pkg.Metadata.Date) > 0 {
		metadata.Date = pkg.Metadata.Date[0]
	}

	if len(pkg.Metadata.Rights) > 0 {
		metadata.Rights = pkg.Metadata.Rights[0]
	}

	return metadata, nil
}

func processDirectory(dirPath string, outputFile string, prettyPrint bool) error {
	// Check if directory exists
	dirInfo, err := os.Stat(dirPath)
	if err != nil {
		return err
	}

	if !dirInfo.IsDir() {
		return fmt.Errorf("%s is not a directory", dirPath)
	}

	// Find all epub files in the directory
	epubFiles := []string{}
	err = filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.ToLower(filepath.Ext(path)) == ".epub" {
			epubFiles = append(epubFiles, path)
		}
		return nil
	})

	if err != nil {
		return err
	}

	if len(epubFiles) == 0 {
		return fmt.Errorf("no epub files found in %s", dirPath)
	}

	// Process each epub file
	var metadataList []BookMetadata
	for _, filePath := range epubFiles {
		metadata, err := extractMetadata(filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error processing %s: %v\n", filePath, err)
			continue
		}
		metadataList = append(metadataList, *metadata)
	}

	// Output the metadata
	var output io.Writer
	if outputFile == "" {
		output = os.Stdout
	} else {
		file, err := os.Create(outputFile)
		if err != nil {
			return err
		}
		defer file.Close()
		output = file
	}

	// Encode to JSON
	encoder := json.NewEncoder(output)
	if prettyPrint {
		encoder.SetIndent("", "  ")
	}
	return encoder.Encode(metadataList)
}

func main() {
	dirPath := flag.String("dir", "", "Directory containing epub files")
	outputFile := flag.String("output", "", "Output JSON file (defaults to stdout)")
	prettyPrint := flag.Bool("pretty", true, "Pretty-print the JSON output")

	flag.Parse()

	if *dirPath == "" {
		if flag.NArg() > 0 {
			*dirPath = flag.Arg(0)
		} else {
			fmt.Println("Usage: epub-metadata-extractor -dir <directory> [-output <file>] [-pretty=true|false]")
			flag.PrintDefaults()
			os.Exit(1)
		}
	}

	if err := processDirectory(*dirPath, *outputFile, *prettyPrint); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
