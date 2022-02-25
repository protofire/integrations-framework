package testreporters

import (
	"crypto/tls"
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/rs/zerolog/log"
	"github.com/smartcontractkit/integrations-framework/utils"
	"gopkg.in/gomail.v2"
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
func (o *OCRSoakTestReporter) WriteReport() error {
	for _, report := range o.Reports {
		report.averageRoundBlocks = report.totalBlockLengths / report.TotalRounds
		report.averageRoundTime = time.Duration(report.totalRoundTimes.Nanoseconds() / int64(report.TotalRounds))
	}
	emailServer := os.Getenv("EMAIL_SERVER")
	emailAddress := os.Getenv("EMAIL_ADDRESS")
	emailPassword := os.Getenv("EMAIL_PASSWORD")
	if err := o.sendEmail(emailServer, emailAddress, emailPassword); err != nil {
		log.Error().Err(err).
			Str("Server", emailServer).
			Str("User", emailAddress).
			Str("Pass", emailPassword).
			Msg("Error trying to send email to notify of soak test ending.")
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
	return nil
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

// sends an email to whatever account is set in framework settings
func (o *OCRSoakTestReporter) sendEmail(emailServer, emailAddress, emailPassword string) error {
	if emailServer == "" ||
		emailAddress == "" ||
		emailPassword == "" {
		return errors.New("Unable to send email: Test Runner email params not set")
	}
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

	mailSender := gomail.NewDialer(
		emailServer,
		587,
		emailAddress,
		emailPassword,
	)
	mailSender.TLSConfig = &tls.Config{InsecureSkipVerify: true} //#nosec G402
	msg := gomail.NewMessage()
	msg.SetHeaders(
		map[string][]string{
			"From": {emailAddress},
			"To":   {emailAddress},
		},
	)
	var emailBody strings.Builder
	emailBody.WriteString("OCR Soak Test summary results are attached in a CSV\n\n")
	formattedTime := time.Now().Format("Mon 3:04PM MST")
	if ginkgo.CurrentSpecReport().Failed() {
		msg.SetHeader("Subject", fmt.Sprintf("OCR Soak Test FAILED at %s", formattedTime))
		emailBody.WriteString(ginkgo.CurrentSpecReport().Failure.Message)
	} else {
		msg.SetHeader("Subject", fmt.Sprintf("OCR Soak Test Passed at %s", formattedTime))
	}
	msg.SetBody("text/plain", emailBody.String())

	msg.Attach(filepath.Join(utils.ProjectRoot, "ocr_soak_report.csv"))
	return mailSender.DialAndSend(msg)
}
