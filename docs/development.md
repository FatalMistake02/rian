# Development

Requires Go 1.26+ and Node.js 22+ for npm tooling. There are no npm dependencies to install.

## Build and checks

```sh
gofmt -w cmd internal
go test ./...
go test -race ./...
go vet ./...
npm test
go build -trimpath -ldflags="-s -w" -o rian ./cmd/rian
./rian --version
```

On Windows, use `-o rian.exe` and run `./rian.exe`. The standalone executable needs neither Go nor Node.js to run.

## npm package

```sh
npm run build
npm run test:package
npm run test:install
npm pack
```

The build creates macOS, Linux, and Windows ARM64/x64 executables under `dist/binaries/`. The install check requires util-linux's `script` command on Linux. To test an existing tarball, use `npm run test:install -- /path/to/package.tgz`.

Go licenses are read from `GOROOT/LICENSE` or the versioned Debian package notice automatically. For other layouts, use `RIAN_GO_LICENSE_FILE=/path/to/LICENSE npm run build`.
