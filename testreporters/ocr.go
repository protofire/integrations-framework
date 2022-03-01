package testreporters

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"text/template"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/rs/zerolog/log"
	"github.com/smartcontractkit/integrations-framework/utils"
)

type OCRSoakTestReporter struct {
	Reports map[string]*OCRSoakTestReport // contractAddress: Report
}

type OCRSoakTestReport struct {
	ContractAddress string
	TotalRounds     uint

	averageRoundTime  time.Duration
	LongestRoundTime  time.Duration
	ShortestRoundTime time.Duration
	totalRoundTimes   time.Duration

	averageRoundBlocks  uint
	LongestRoundBlocks  uint
	ShortestRoundBlocks uint
	totalBlockLengths   uint
}

// WriteReport writes OCR Soak test report to logs
// TODO: Need to generally improve log creation for soak tests. Would like this to be a CSV or Grafana or something
func (o *OCRSoakTestReporter) WriteReport(namespace string) {
	for _, report := range o.Reports {
		report.averageRoundBlocks = report.totalBlockLengths / report.TotalRounds
		report.averageRoundTime = time.Duration(report.totalRoundTimes.Nanoseconds() / int64(report.TotalRounds))
	}
	csvPath := filepath.Join(utils.ProjectRoot, "ocr_soak_report.csv")
	if err := o.writeCSV(csvPath); err != nil {
		log.Error().Err(err).Str("Path", csvPath).Msg("Error writing CSV file")
	}
	if err := o.sendSlack(csvPath, namespace); err != nil {
		log.Error().Err(err).Msg("Error sending slack message to webhook")
	}

	log.Info().Msg("OCR Soak Test Report")
	log.Info().Msg("--------------------")
	for contractAddress, report := range o.Reports {
		log.Info().
			Str("Contract Address", report.ContractAddress).
			Uint("Total Rounds Processed", report.TotalRounds).
			Str("Average Round Time", fmt.Sprint(report.averageRoundTime)).
			Str("Longest Round Time", fmt.Sprint(report.LongestRoundTime)).
			Str("Shortest Round Time", fmt.Sprint(report.ShortestRoundTime)).
			Uint("Average Round Blocks", report.averageRoundBlocks).
			Uint("Longest Round Blocks", report.LongestRoundBlocks).
			Uint("Shortest Round Blocks", report.ShortestRoundBlocks).
			Msg(contractAddress)
	}
	log.Info().Msg("--------------------")
}

// UpdateReport updates the report based on the latest info
func (o *OCRSoakTestReport) UpdateReport(roundTime time.Duration, blockLength uint) {
	// Updates min values from default 0
	if o.ShortestRoundBlocks == 0 {
		o.ShortestRoundBlocks = blockLength
	}
	if o.ShortestRoundTime == 0 {
		o.ShortestRoundTime = roundTime
	}
	o.TotalRounds++
	o.totalRoundTimes += roundTime
	o.totalBlockLengths += blockLength
	if roundTime >= o.LongestRoundTime {
		o.LongestRoundTime = roundTime
	}
	if roundTime <= o.ShortestRoundTime {
		o.ShortestRoundTime = roundTime
	}
	if blockLength >= o.LongestRoundBlocks {
		o.LongestRoundBlocks = blockLength
	}
	if blockLength <= o.ShortestRoundBlocks {
		o.ShortestRoundBlocks = blockLength
	}
}

// writes a CSV report on the test runner
func (o *OCRSoakTestReporter) writeCSV(csvPath string) error {
	ocrReportFile, err := os.Create(filepath.Join(utils.ProjectRoot, "ocr_soak_report.csv"))
	if err != nil {
		return err
	}
	defer ocrReportFile.Close()

	ocrReportWriter := csv.NewWriter(ocrReportFile)
	err = ocrReportWriter.Write([]string{
		"Contract Index",
		"Contract Address",
		"Total Rounds Processed",
		"Average Round Time",
		"Longest Round Time",
		"Shortest Round Time",
		"Average Round Blocks",
		"Longest Round Blocks",
		"Shortest Round Blocks",
	})
	if err != nil {
		return err
	}
	for contractIndex, report := range o.Reports {
		err = ocrReportWriter.Write([]string{
			fmt.Sprint(contractIndex),
			report.ContractAddress,
			fmt.Sprint(report.TotalRounds),
			fmt.Sprint(report.averageRoundTime),
			fmt.Sprint(report.LongestRoundTime),
			fmt.Sprint(report.ShortestRoundTime),
			fmt.Sprint(report.averageRoundBlocks),
			fmt.Sprint(report.LongestRoundBlocks),
			fmt.Sprint(report.ShortestRoundBlocks),
		})
		if err != nil {
			return err
		}
	}
	ocrReportWriter.Flush()

	log.Info().Str("Location", filepath.Join(utils.ProjectRoot, "ocr_soak_report.csv")).Msg("Wrote CSV file")
	return nil
}

// sends a slack message to a slack webhook if one is provided
func (o *OCRSoakTestReporter) sendSlack(csvLocation, namespace string) error {
	slackWebhook := os.Getenv("SLACK_WEBHOOK")
	if slackWebhook == "" ||
		slackWebhook == "https://hooks.slack.com/services/XXX" ||
		slackWebhook == "https://hooks.slack.com/services/" {
		return fmt.Errorf("Unable to send slack notification, Webhook not set '%s'", slackWebhook)
	}

	// Marshal values
	slackMessageVars := map[string]interface{}{
		"NameSpace":    namespace,
		"TestDuration": ginkgo.CurrentSpecReport().RunTime,
		"CSVLocation":  csvLocation,
		"ErrorTrace":   ginkgo.CurrentSpecReport().FailureMessage(),
	}
	var buf bytes.Buffer
	tmpl := template.New("slack")

	if ginkgo.CurrentSpecReport().Failed() {
		tmpl.Parse(soakTestFailureSlack)
	} else {
		tmpl.Parse(soakTestSuccessSlack)
	}
	tmpl.Execute(&buf, slackMessageVars)

	req, err := http.NewRequest(http.MethodPost, slackWebhook, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Got invalid status code '%d' from webhook call. Expected '%d'", resp.StatusCode, http.StatusOK)
	}
	return nil
}

var soakTestFailureSlack = `{
	"blocks": [
		{
			"type": "header",
			"text": {
				"type": "plain_text",
				"text": ":x: OCR Soak Test FAILED :x:"
			}
		},
		{
			"type": "context",
			"elements": [
				{
					"type": "plain_text",
					"text": "{{ .NameSpace }}"
				}
			]
		},
		{
			"type": "divider"
		},
		{
			"type": "section",
			"text": {
				"type": "mrkdwn",
				"text": "Test ran for {{ .TestDuration }}\nSummary CSV created on __remote-test-runner__ at __{{ .CSVLocation }}__"
			}
		},
		{
			"type": "header",
			"text": {
				"type": "plain_text",
				"text": "Error Trace"
			}
		},
		{
			"type": "divider"
		},
		{
			"type": "section",
			"text": {
				"type": "plain_text",
				"text": "{{ .ErrorTrace }}"
			}
		}
	]
}`

var soakTestSuccessSlack = `{
	"blocks": [
		{
			"type": "header",
			"text": {
				"type": "plain_text",
				"text": ":white_check_mark: OCR Soak Test PASSED :white_check_mark:"
			}
		},
		{
			"type": "context",
			"elements": [
				{
					"type": "plain_text",
					"text": "{{ .NameSpace }}"
				}
			]
		},
		{
			"type": "divider"
		},
		{
			"type": "section",
			"text": {
				"type": "mrkdwn",
				"text": "Test ran for {{ .TestDuration }}\nSummary CSV created on _remote-test-runner_ at _{{ .CSVLocation }}_"
			}
		}
	]
}`
