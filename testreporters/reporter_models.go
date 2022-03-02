// Package testreporters holds all the tools necessary to report on tests that are run utilizing the testsetups package
package testreporters

// TestReporter is a general interface for all test reporters
type TestReporter interface {
	WriteReport(folderPath string) error
}
