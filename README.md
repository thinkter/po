# po

`po` is a CLI tool designed to be an abstraction layer on top of Git and GitHub, making it incredibly easy to set up and use version control.

## Prerequisites

`po` relies on the system's `git` installation. Please ensure `git` is installed and available in your system's PATH.

## Development

### Running from source

You can run the application directly using Go:

```bash
go run cmd/po/main.go
```

### Building

To build the binary:

```bash
go build -o po cmd/po/main.go
```

## Usage

Currently, running `po` performs a system check to verify that `git` is installed and accessible.

```bash
./po
```

Output on success:
```
git is installed! Ready to rock.
```
