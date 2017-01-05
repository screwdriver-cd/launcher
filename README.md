# Screwdriver Job Launcher
[![Build Status][build-image]][build-url]
[![Open Issues][issues-image]][issues-url]
[![Latest Release][version-image]][version-url]
[![Go Report Card][goreport-image]][goreport-url]

> The entrypoint for job launching in Screwdriver

## Usage

```bash
$ go get github.com/screwdriver-cd/launcher
$ launch --api-uri http://localhost:8080/v4 123456
```

## Testing

```bash
$ go get github.com/screwdriver-cd/launcher
$ go test -cover github.com/screwdriver-cd/launcher/...
```

## License

Code licensed under the BSD 3-Clause license. See LICENSE file for terms.

[version-image]: https://img.shields.io/github/tag/screwdriver-cd/launcher.svg
[version-url]: https://github.com/screwdriver-cd/launcher/releases
[issues-image]: https://img.shields.io/github/issues/screwdriver-cd/screwdriver.svg
[issues-url]: https://github.com/screwdriver-cd/screwdriver/issues
[build-image]: https://cd.screwdriver.cd/pipelines/21/badge
[build-url]: https://cd.screwdriver.cd/pipelines/21
[goreport-image]: https://goreportcard.com/badge/github.com/Screwdriver-cd/launcher
[goreport-url]: https://goreportcard.com/report/github.com/Screwdriver-cd/launcher
