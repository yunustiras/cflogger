package pkg

import (
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"
)

var FinalStatusArray = []string{"CREATE_COMPLETE", "CREATE_FAILED",
	"DELETE_COMPLETE", "DELETE_COMPLETE",
	"ROLLBACK_COMPLETE", "ROLLBACK_FAILED",
	"UPDATE_COMPLETE", "UPDATE_FAILED",
	"UPDATE_ROLLBACK_COMPLETE", "UPDATE_ROLLBACK_FAILED",
	"IMPORT_COMPLETE", "IMPORT_FAILED",
	"IMPORT_ROLLBACK_COMPLETE", "IMPORT_ROLLBACK_FAILED",
}

var Index int = 0
var StartTime = time.Now().Add(-1 * time.Second)
var timeout time.Duration
var data string
var region string

func Start() {

	currentChannel := make(chan string, 1)

	sess, err := session.NewSession()
	errorHandle(err)
	svc := cloudformation.New(sess, &aws.Config{
		Region: &region,
	})

	go func() {
		text := handler(svc, data, StartTime)
		currentChannel <- text
	}()

	select {
	case <-currentChannel:

	case <-time.After(timeout):
		fmt.Println("Timeout reached.")
	}

}

func handler(svc *cloudformation.CloudFormation, data string, StartTime time.Time) string {
	for {
		e, err := getEvents(svc, data, StartTime)
		errorHandle(err)
		cont := getStatus(reverseSlice(e))
		if !cont {
			break
		}
		time.Sleep(5 * time.Second)
	}
	return ""

}

// Get StackEvents.
func getEvents(cf *cloudformation.CloudFormation, StackId string, StartTime time.Time) ([]*cloudformation.StackEvent, error) {

	var err error
	var events []*cloudformation.StackEvent
	var nextToken *string
	for {
		params := &cloudformation.DescribeStackEventsInput{
			StackName: aws.String(StackId),
			NextToken: nextToken,
		}
		resp, err := cf.DescribeStackEvents(params)
		if err != nil {
			// Implement exponential backoff on throttling error
			if isThrottlingError(err) {
				time.Sleep(getBackoffDuration())
				continue
			}
			return nil, err
		}
		// Get Last Triggered Events
		for _, e := range resp.StackEvents {
			if e.Timestamp.After(StartTime) {
				events = append(events, e)
			}
		}
		if resp.NextToken == nil {
			break
		} else {
			nextToken = resp.NextToken
		}
	}
	if len(events) == 0 {
		err = errors.New("Error! There are no events in the specified stack.")
	}
	return events, err
}

// Lookup State until Stackupdate/StackCreate/Stackdelete completed.
func getStatus(input []*cloudformation.StackEvent) bool {
	cont := true

	if len(input) > Index {
		for j, e := range input[Index:] {
			writer := customWriter(aws.StringValue(e.ResourceStatus))
			writer.Println(aws.Time(*e.Timestamp), aws.StringValue(e.LogicalResourceId), aws.StringValue(e.ResourceStatus), aws.StringValue(e.ResourceStatusReason))

			if aws.StringValue(e.ResourceType) == "AWS::CloudFormation::Stack" {
				for _, s := range FinalStatusArray {
					if aws.StringValue(e.ResourceStatus) == s {
						cont = false
						break
					}
				}
			}
			if !cont {
				break
			}
		}
		Index = len(input)
	}

	return cont
}

func isThrottlingError(err error) bool {
	return err != nil && (err.Error() == "Throttling" || err.Error() == "Rate exceeded")
}

func getBackoffDuration() time.Duration {

	backoffDuration := time.Second * 2
	maxBackoff := time.Minute * 2

	
	for i := 0; i < 5; i++ {
		backoffDuration *= 2
		if backoffDuration > maxBackoff {
			return maxBackoff
		}
	}
	return backoffDuration
}
