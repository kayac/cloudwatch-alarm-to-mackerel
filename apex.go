/*
Lambda function to alert the Cloudwatch Alarm to mackerel.
*/
package cwa2mkr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/apex/go-apex/sns"
	"github.com/aws/aws-lambda-go/lambda"
)

const (
	checkReportEndpoint = "https://api.mackerelio.com/api/v0/monitoring/checks/report"
	reportMsgFmt        = "%s status is '%s', reason: %s, alarm_description: %s, state_change_time: %s, metrics: %s, namespace: %s"

	statusOK       = "OK"
	statusWarning  = "WARNING"
	statusCritical = "CRITICAL"
)

// https://mackerel.io/ja/api-docs/entry/check-monitoring
//
// json struct should be posted:
// {
//   "reports": [
//     {
//       "source": {
//         "type": "host",
//         "hostId": "hostid"
//       },
//       "name": "Mycron Batch Failed",
//       "status": "CRITICAL",
//       "message": "alert message",
//       "occurredAt": epoch_time
//     }
//   ]
// }
type Reports struct {
	Reports []Report `json:"reports"`
}

type Report struct {
	// source struct reference
	Source source `json:"source"`

	// monitoring name
	Name string `json:"name"`

	// result of status: "OK", "CRITICAL", "WARNING", "UNKNOWN"
	Status string `json:"status"` // OK, ALARM

	// message memo, 1024 characters
	Message string `json:"message"`

	// monitor time (epoch sec)
	OccurredAt int64 `json:"occurredAt"`

	// [optional] alert resent interval(min). default is not resending, and if it is less than 10 min, it is set 10 min.
	NotificationInterval int `json:"notificationInterval,omitempty"`
}

type source struct {
	// constant string "host"
	Type string `json:"type"`

	// mackerel host id
	HostID string `json:"hostId"`
}

// a content of record sent to lambd by SNS:
// {
//   "AlarmName": "test",
//   "AlarmDescription": "test",
//   "AWSAccountId": "***",
//   "NewStateValue": "OK",
//   "NewStateReason": "Threshold Crossed: no datapoints were received for 1 period and 1 missing datapoint was treated as [NonBreaching].",
//   "StateChangeTime": "2018-02-16T08:42:33.109+0000",
//   "Region": "Asia Pacific (Tokyo)",
//   "OldStateValue": "ALARM",
//   "Trigger": {
//     "MetricName": "FailedInvocations",
//     "Namespace": "AWS/Events",
//     "StatisticType": "Statistic",
//     "Statistic": "SUM",
//     "Unit": null,
//     "Dimensions": [
//       {
//         "name": "RuleName",
//         "value": "cron_name",
//       }
//     ],
//     "Period": 60,
//     "EvaluationPeriods": 1,
//     "ComparisonOperator": "GreaterThanOrEqualToThreshold",
//     "Threshold": 0,
//     "TreatMissingData": "- TreatMissingData: NonBreaching",
//     "EvaluateLowSampleCountPercentile": ""
//   }
// }
//
type snsMessage struct {
	AlarmName        string  `json:"AlarmName"`
	AlarmDescription string  `json:"AlarmDescription"`
	NewStateValue    string  `json:"NewStateValue"`
	NewStateReason   string  `json:"NewStateReason"`
	StateChangeTime  string  `json:"StateChangeTime"`
	Trigger          trigger `json:"Trigger"`
}

type trigger struct {
	MetricName string `json:"MetricName"`
	Namespace  string `json:"NameSpace"`
}

func (m snsMessage) toMackerelStatus() string {
	if m.NewStateValue == statusOK {
		return statusOK
	}
	if strings.HasPrefix(m.AlarmDescription, "CRITICAL") {
		return statusCritical
	}
	return statusWarning
}

func ApexRun() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	apiKey, hostID, err := parseEnvVars()
	if err != nil {
		return err
	}

	handler := func(ctx context.Context, event *sns.Event) error {
		reps := Reports{
			Reports: make([]Report, 0, len(event.Records)),
		}

		for _, record := range event.Records {
			var msg snsMessage
			if err := json.Unmarshal([]byte(record.SNS.Message), &msg); err != nil {
				log.Println(err)
				continue
			}

			// empty is not expected, so skip.
			if msg.AlarmName == "" || msg.NewStateValue == "" {
				log.Printf("got the unknown message: %#v", msg)
				continue
			}

			reps.Reports = append(reps.Reports, Report{
				Source: source{
					HostID: hostID,
					Type:   "host",
				},
				Name:   msg.AlarmName,
				Status: msg.toMackerelStatus(),
				Message: fmt.Sprintf(reportMsgFmt,
					msg.AlarmName,
					msg.NewStateValue,
					msg.NewStateReason,
					msg.AlarmDescription,
					msg.StateChangeTime,
					msg.Trigger.MetricName,
					msg.Trigger.Namespace,
				),
				OccurredAt: time.Now().Unix(),
			})
		}

		return PostChecksReport(apiKey, reps)
	}

	lambda.Start(handler)

	return nil
}

func parseEnvVars() (apiKey, hostID string, err error) {
	if hostID = os.Getenv("HOST_ID"); hostID == "" {
		err = errors.New("HOST_ID is required")
		return
	}

	if apiKey = os.Getenv("MACKEREL_APIKEY"); apiKey == "" {
		err = errors.New("MACKEREL_APIKEY is required")
		return
	}

	return
}

func PostChecksReport(apiKey string, reps Reports) error {
	body := new(bytes.Buffer)
	if err := json.NewEncoder(body).Encode(reps); err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, checkReportEndpoint, body)
	if err != nil {
		return err
	}

	req.Header.Set("Content-type", "application/json")
	req.Header.Set("X-Api-Key", apiKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if status := resp.StatusCode; status >= 400 {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: status code %d %s", status, err)
		}
		return fmt.Errorf("failed to post: status code %d %s", status, string(body))
	}

	return nil
}
