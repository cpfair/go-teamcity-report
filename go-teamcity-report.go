// Copyright (c) 2016 All Rights Reserved, Improbable Worlds Ltd.

package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// This converts standard Go test output to be all pretty in TeamCity
// TeamCity reporting format: https://confluence.jetbrains.com/display/TCD7/Build+Script+Interaction+with+TeamCity#BuildScriptInteractionwithTeamCity-ReportingTests
// Go test output is of the following form:
//
// === RUN testname
// --- (PASS|FAIL|SKIP): testname (1.23s)
// [failure output if applicable]
// ...
// (PASS|FAIL) [appears at the end of a succesful package of tests]
// (ok|FAIL|?) packagename (4.56s)
//
// Unfortunately, stdout just gets plastered wherever, especially during parallel tests. Yay go?
// Also unfortunately, we can't report completely realtime since we don't know the package name until it completes.

var (
	// For parsing
	testRunPattern       = regexp.MustCompile(`^=== RUN\s+(\S+)`)
	testFinishPattern    = regexp.MustCompile(`^--- (PASS|FAIL|SKIP):\s+(\S+) \(([\d.]+)s\)`)
	packageFinishPattern = regexp.MustCompile(`^(ok|FAIL|\?)\s+(\S+)`)
	cruftPattern         = regexp.MustCompile(`^(PASS|FAIL)$`)
	// For escaping
	specialCharsPattern  = regexp.MustCompile(`\n|\r|\[|\]|\||'`)
	nonAsciiCharsPattern = regexp.MustCompile(`[\x00-\x20]|[\x80-\x{ffff}]`)
)

type testResult struct {
	name        string
	status      string
	output      []string
	durationSec float64
}

func escape(input string) string {
	// TC escaping is described here https://confluence.jetbrains.com/display/TCD7/Build+Script+Interaction+with+TeamCity#BuildScriptInteractionwithTeamCity-servMsgsServiceMessages
	specEscape := func(in string) string {
		if in == "\n" {
			return "|n"
		} else if in == "\r" {
			return "|r"
		} else {
			return "|" + in
		}
	}
	input = specialCharsPattern.ReplaceAllStringFunc(input, specEscape)
	unicodeEscape := func(in string) string {
		return fmt.Sprintf("|0x%04x", byte(in[0]))
	}
	return nonAsciiCharsPattern.ReplaceAllStringFunc(input, unicodeEscape)
}

func (test *testResult) flush() {
	fmt.Printf("##teamcity[testStarted name='%s' captureStandardOutput='true']\n", escape(test.name))
	testOutput := strings.Join(test.output, "\n")
	if len(test.output) > 0 {
		fmt.Println(testOutput)
	}
	if test.status == "PASS" {
		// There is no testSucceeded message in TC
	} else if test.status == "FAIL" {
		// We need a message for TC to properly recognize the failure
		// So, try to come up with something succinct
		message := regexp.MustCompile(`(?m)Error:\s+(.+)$`).FindString(testOutput)
		if len(message) == 0 {
			message = strings.TrimSpace(strings.Split(testOutput, "\n")[0])
		}
		fmt.Printf("##teamcity[testFailed name='%s' message='%s']\n", escape(test.name), escape(message))
	} else if test.status == "SKIP" {
		fmt.Printf("##teamcity[testIgnored name='%s']\n", escape(test.name))
	}
	fmt.Printf("##teamcity[testFinished name='%s' duration='%d']\n", escape(test.name), int(test.durationSec*1000))
}

func flushPackage(name string, results []*testResult) {
	fmt.Printf("##teamcity[testSuiteStarted name='%s']\n", escape(name))
	for _, test := range results {
		test.flush()
	}
	fmt.Printf("##teamcity[testSuiteFinished name='%s']\n", escape(name))
}

func findTest(name string, results []*testResult) *testResult {
	for _, test := range results {
		if test.name == name {
			return test
		}
	}
	return nil
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	// We hold onto the test results for a package until it completes, so we can properly output it as a suite
	packageTestBuffer := []*testResult{}
	// We explicitly capture test output only upon failure, otherwise it is passed through immediately.
	var capturingTest *testResult
	for scanner.Scan() {
		input := scanner.Text()

		if cruftPattern.MatchString(input) {
			// Some stuff we just want to drop
		} else if match := testRunPattern.FindStringSubmatch(input); match != nil {
			capturingTest = nil
			packageTestBuffer = append(packageTestBuffer, &testResult{name: match[1]})
		} else if match := testFinishPattern.FindStringSubmatch(input); match != nil {
			test := findTest(match[2], packageTestBuffer)
			if test == nil {
				panic("Run `go test` with -v")
			}
			test.durationSec, _ = strconv.ParseFloat(match[3], 32)
			test.status = match[1]
			if test.status == "FAIL" {
				// Failure output proceeds a test failure header
				capturingTest = test
			}
		} else if match := packageFinishPattern.FindStringSubmatch(input); match != nil {
			capturingTest = nil
			// Flush package results
			flushPackage(match[2], packageTestBuffer)
			packageTestBuffer = []*testResult{}
		} else if capturingTest != nil {
			// Capture output to the current test
			capturingTest.output = append(capturingTest.output, input)
		} else {
			// Who knows
			fmt.Println(input)
		}
	}
}
