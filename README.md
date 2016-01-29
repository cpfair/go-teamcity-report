# go-teamcity-report

Converts `go test` output for TeamCity reporting.

## Installation

    go get -u github.com/cpfair/go-teamcity-report

## Usage

    set -o pipefail # Otherwise `go-teamcity-report` will swallow the exit code of `go test`
    go test -v | go-teamcity-report
