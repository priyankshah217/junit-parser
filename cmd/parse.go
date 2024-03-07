package cmd

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

var filePath string

type TestSuites struct {
	XMLName   xml.Name `xml:"testsuites"`
	Text      string   `xml:",chardata"`
	Tests     string   `xml:"tests,attr"`
	Failures  string   `xml:"failures,attr"`
	Errors    string   `xml:"errors,attr"`
	Time      string   `xml:"time,attr"`
	TestSuite []struct {
		Text       string `xml:",chardata"`
		Tests      string `xml:"tests,attr"`
		Failures   string `xml:"failures,attr"`
		Time       string `xml:"time,attr"`
		Name       string `xml:"name,attr"`
		Timestamp  string `xml:"timestamp,attr"`
		Properties struct {
			Text     string `xml:",chardata"`
			Property struct {
				Text  string `xml:",chardata"`
				Name  string `xml:"name,attr"`
				Value string `xml:"value,attr"`
			} `xml:"property"`
		} `xml:"properties"`
		Testcase []struct {
			Text      string `xml:",chardata"`
			Classname string `xml:"classname,attr"`
			Name      string `xml:"name,attr"`
			Time      string `xml:"time,attr"`
			Failure   struct {
				Text    string `xml:",chardata"`
				Message string `xml:"message,attr"`
				Type    string `xml:"type,attr"`
			} `xml:"failure"`
		} `xml:"testcase"`
	} `xml:"testsuite"`
}

type Issue int

const (
	assertionIssue = iota
	scriptIssue
	infraIssue
	configOrDBIssue
)

func (i Issue) String() string {
	return [...]string{"Assertion", "Script", "Infrastructure", "Config Or DB"}[i]
}

type RootCause struct {
	issueType        Issue
	rootCauseMessage string
}

type TestResult struct {
	testCaseName string
	rootCause    *RootCause
}

// ReadJunitXml Read Junit XML from given path
func ReadJunitXml(path string) (*TestSuites, error) {
	var testSuites *TestSuites
	file, err := os.Open(path)
	if err != nil {
		return testSuites, err
	}
	defer file.Close()
	byteValue, _ := io.ReadAll(file)
	err = xml.Unmarshal(byteValue, &testSuites)
	if err != nil {
		return nil, err
	}
	return testSuites, nil
}

// parseCmd represents the parse command
var parseCmd = &cobra.Command{
	Use:   "parse",
	Short: "parse is a tool to parse junit xml files and output the results in a human readable format.",
	Run: func(cmd *cobra.Command, args []string) {
		var testResults []*TestResult
		testSuites, err := ReadJunitXml(filePath)
		if err != nil {
			fmt.Println(err)
		}
		for _, testSuite := range testSuites.TestSuite {
			if testSuite.Failures != "0" {
				for _, testCase := range testSuite.Testcase {
					if testCase.Failure.Message != "" && strings.Contains(testCase.Name, "Test_") {
						testCaseName, rootCauseInfo := testCase.Name, getRootCause(testCase.Failure.Text)
						testResults = append(testResults, &TestResult{testCaseName, rootCauseInfo})
					}
				}
			}
		}
		for i := range testResults {
			cmd.Printf("%d ------------- Test Case Name: %s ------------- \n", i+1, testResults[i].testCaseName)
			cmd.Printf("Root Cause: %s \n", testResults[i].rootCause.rootCauseMessage)
			cmd.Printf("Issue Type: %s \n", testResults[i].rootCause.issueType)
			cmd.Println("---------------------------------------------------")
		}
	},
}

func init() {
	rootCmd.AddCommand(parseCmd)
	parseCmd.Flags().StringVarP(&filePath, "file-path", "f", "", "The input file to parse")
}

// getRootCause Get service URLs from failure message
func getRootCause(failureMessage string) *RootCause {
	rootCause := &RootCause{}
	newMsg := strings.Trim(failureMessage, "\t\n\v\f\r ")
	errMessageRegEx := regexp.MustCompile(`Error:(.*?)\n|\r|\t.+Test:`)
	errMessage := errMessageRegEx.FindStringSubmatch(newMsg)
	if len(errMessage) > 0 {
		s := errMessage[1]
		var msg string
		if strings.Contains(s, "HTTP status code is not in the range 200~299, but the response is not nil") {
			msg = strings.ReplaceAll(s, "HTTP status code is not in the range 200~299, but the response is not nil,", "")
			rootCause.issueType = infraIssue
		} else {
			rootCause.issueType = assertionIssue
			if strings.Contains(s, " &errors.errorString{") {
				rootCause.issueType = configOrDBIssue
			}
			msg = s
		}
		msg = strings.Trim(msg, "\t\n\v\f\r ")
		rootCause.rootCauseMessage = strings.ReplaceAll(msg, "s:\"", "")
		return rootCause
	}
	rootCause.rootCauseMessage = "panic in execution"
	rootCause.issueType = scriptIssue
	return rootCause
}
