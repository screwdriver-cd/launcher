# Screwdriver Launcher
[![Build Status][build-image]][build-url]
[![Open Issues][issues-image]][issues-url]
[![Latest Release][version-image]][version-url]
[![Go Report Card][goreport-image]][goreport-url]

> The entrypoint for launching job in Screwdriver

## Usage

### Go

```bash
$ go get github.com/screwdriver-cd/launcher
$ launcher --api-uri http://localhost:8080/v4 buildId
```

### Habitat

```bash
$ hab package install screwdriver-cd/launcher
$ hab package exec screwdriver-cd/launcher launcher --api-uri http://localhost:8080/v4 buildId
```

### Docker

```bash
$ docker pull screwdrivercd/launcher
$ docker run screwdrivercd/launcher --api-uri http://localhost:8080/v4 buildId
```

If you want to use an alternative shell (instead of `/bin/sh`) you can set the environment variable
`SD_SHELL_BIN` to what you want to use.

```bash
$ SD_SHELL_BIN=/bin/bash launch --api-url http://localhost:8080/v4 buildId
```

## Testing

```bash
$ go get github.com/screwdriver-cd/launcher
$ go test -cover github.com/screwdriver-cd/launcher/...
```

## Building

### Habitat

```bash
$ habitat studio enter
$ build
$ hab pkg exec $HAB_ORIGIN/launcher launcher --help
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
