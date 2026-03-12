package main

import (
	"fmt"
	"os"

	"github.com/slack-go/slack"
)

func notifySlack(name string, namespace string, environment string, tag string, oldtag string, slackmoji string, tagMessage string) string {
	api := slack.New(os.Getenv("SLACK_TOKEN"))
	text := fmt.Sprintf("%s %s\n*%s*  (was %s)\n%s", name, environment, tag, oldtag, tagMessage)

	channelID, timestamp, err := api.PostMessage(
		os.Getenv("SLACK_CHANNEL"),
		slack.MsgOptionText(text, false),
		slack.MsgOptionIconEmoji(fmt.Sprintf(":%s:", slackmoji)),
	)
	if err != nil {
		fmt.Printf("%s\n", err)
		return ""
	}
	fmt.Printf("Message successfully sent to channel %s at %s\n", channelID, timestamp)
	return timestamp
}
