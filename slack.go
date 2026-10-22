package main

import (
	"fmt"
	"os"

	"github.com/slack-go/slack"
)

func notifySlack(name string, namespace string, environment string, tag string, oldtag string, slackmoji string, tagMessage string) string {
	api := slack.New(os.Getenv("SLACK_TOKEN"))
	text := fmt.Sprintf("%s %s\n*%s*  (was %s)", name, environment, tag, oldtag)
	attachment := slack.Attachment{
		Text: tagMessage,
	}

	channelID, timestamp, err := api.PostMessage(
		os.Getenv("SLACK_CHANNEL"),
		slack.MsgOptionText(text, false),
		slack.MsgOptionAttachments(attachment),
		slack.MsgOptionIconEmoji(fmt.Sprintf(":%s:", slackmoji)),
	)
	if err != nil {
		fmt.Printf("%s\n", err)
		return ""
	}
	fmt.Printf("Message successfully sent to channel %s at %s\n", channelID, timestamp)
	return timestamp
}

// replySlack posts text in the thread of the deploy message at threadTs.
func replySlack(threadTs string, text string) {
	if threadTs == "" {
		fmt.Println("No Slack thread to reply in")
		return
	}
	api := slack.New(os.Getenv("SLACK_TOKEN"))
	_, _, err := api.PostMessage(
		os.Getenv("SLACK_CHANNEL"),
		slack.MsgOptionText(text, false),
		slack.MsgOptionTS(threadTs),
	)
	if err != nil {
		fmt.Printf("%s\n", err)
	}
}
