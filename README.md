# Magick Server

A simple API server to convert (multi-page) images using ImageMagick.

## Usage

Start the server:

```bash
# Listen on port 8081
go run main.go --listen=:8081
```

The server exposes three endpoints:

- `/health` responds with a JSON status.
- `/version` responds with the Git version used to build the server.
- `/convert` converts a (multi-page) image into a Zip archive of single images.

## Image Conversion

The `/convert` endpoint can take one or multiple of the following options (as URL parameters):

- `density` will set the rendering resolution in DPI (useful for PDF input). Default is `300.0`.
- `quality` will set the compression quality for the output images (useful for JPEG output). Default is `85`.
- `format` will set the output format, either `JPEG`, `PNG`, or `TIFF`. Default it `JPEG`.
- `layout` will set the output layout, either `landscape`, `portrait`, or `keep`. Default is `keep`.

## Development on macOS

```bash
# Install ImageMagick v6 (keg-only)
brew install imagemagick@6

# Compile ImageMagick Go bindings
PKG_CONFIG_PATH="/opt/homebrew/opt/imagemagick@6/lib/pkgconfig" CGO_CFLAGS_ALLOW=-Xpreprocessor go install
```
