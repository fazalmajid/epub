# epub
Small go utility to dump metadata in JSON format for all ePub files in a directory and subdirectories

Built with a fair bit of help from Claude Sonnet 3.7

## Building

```
git clone https://github.com/fazalmajid/epub
cd epub
go build
```

if you just want the utility:

```
go install github.com/fazalmajid/epub@latest
```

## Usage

```
epub -dir <Directory where ePub files reside>
```

## Example

Find all duplicate titles in my library:

```
epub -dir ~/Documents/eBooks | jq 'group_by(.title) | map(select(length > 1)) | flatten'
```
