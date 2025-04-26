# epub
Small go utility to dump metadata in JSON format for all ePub files in a directory and subdirectories

# Building

```
go build
```

# Usage

```
epub -dir <Directory where ePub files reside>
```

# Example

Find all duplicate titles in my library:

```
epub -dir ~/Documents/eBooks | jq 'group_by(.title) | map(select(length > 1)) | flatten'
```
