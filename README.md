# Screwdriver Job Launcher
[![Build Status][build-image]][build-url]
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
[build-image]: https://cd.screwdriver.cd/pipelines/6172e25fa190fe5e23b3685f6e211bd9d066e578/badge
[build-url]: https://cd.screwdriver.cd/pipelines/6172e25fa190fe5e23b3685f6e211bd9d066e578
[goreport-image]: https://goreportcard.com/badge/github.com/Screwdriver-cd/launcher
[goreport-url]: https://goreportcard.com/report/github.com/Screwdriver-cd/launcher
