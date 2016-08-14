# Screwdriver Job Launcher
[![Build Status][wercker-image]][wercker-url] [![Open Issues][issues-image]][issues-url]
[![Go Report Card][goreport-image]][goreport-url]

> The entrypoint for job launching in Screwdriver.

## Usage

```bash
$ go get github.com/screwdriver-cd/launcher
$ launch --api-uri http://localhost:8080/v3 cba94a05f8aa063f4b8cfb62cbc355e0c5f02698
```

## Testing

```bash
$ go get github.com/screwdriver-cd/launcher
$ go test -cover github.com/screwdriver-cd/launcher/...
```

## License

Code licensed under the BSD 3-Clause license. See LICENSE file for terms.

[issues-image]: https://img.shields.io/github/issues/screwdriver-cd/launcher.svg
[issues-url]: https://github.com/screwdriver-cd/launcher/issues
[wercker-image]: https://app.wercker.com/status/822503b7af879d54018006aeafb317ae
[wercker-url]: https://app.wercker.com/project/bykey/822503b7af879d54018006aeafb317ae
[goreport-image]: https://goreportcard.com/badge/github.com/Screwdriver-cd/launcher
[goreport-url]: https://goreportcard.com/report/github.com/Screwdriver-cd/launcher
