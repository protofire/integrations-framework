package testreporters

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"
	"github.com/slack-go/slack"
	"github.com/smartcontractkit/chainlink-testing-framework/client"
	"golang.org/x/sync/errgroup"
)

type ChainlinkProfileTestReporter struct {
	Results   []*client.ChainlinkProfileResults
	namespace string
}

// SetNamespace sets the namespace of the report for clean reports
func (c *ChainlinkProfileTestReporter) SetNamespace(namespace string) {
	c.namespace = namespace
}

// WriteReport create the profile files
func (c *ChainlinkProfileTestReporter) WriteReport(folderLocation string) error {
	profFiles := new(errgroup.Group)
	for _, res := range c.Results {
		result := res
		profFiles.Go(func() error {
			filePath := filepath.Join(folderLocation, fmt.Sprintf("chainlink-node-%d-profiles", result.NodeIndex))
			if err := mkdirIfNotExists(filePath); err != nil {
				return err
			}
			for _, rep := range result.Reports {
				report := rep
				reportFile, err := os.Create(filepath.Join(filePath, report.Type))
				if err != nil {
					return err
				}
				if _, err = reportFile.Write(report.Data); err != nil {
					return err
				}
				if err = reportFile.Close(); err != nil {
					return err
				}
			}
			return nil
		})
	}
	return profFiles.Wait()
}

// SendNotification hasn't been implemented for this test
func (c *ChainlinkProfileTestReporter) SendSlackNotification(slackClient *slack.Client) error {
	log.Warn().Msg("No Slack notification integration for Chainlink profile tests")
	return nil
}
