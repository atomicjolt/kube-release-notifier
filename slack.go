package main

import (
    "fmt"
        "os"

    "github.com/slack-go/slack"
)

func notifySlack(name string, namespace string, environment string, tag string, oldtag string, slackmoji string, tagMessage string) {
    fallback := fmt.Sprintf("Deployed %s %s %s", name, environment, tag)
    api := slack.New(os.Getenv("SLACK_TOKEN"))
    attachment := slack.Attachment{
        Pretext: fmt.Sprintf("New deployment :%s:", slackmoji),
        Fallback: fallback,
        Fields: []slack.AttachmentField{
            slack.AttachmentField{
                Title: "App",
                Value: name,
		Short: false,
            },
            slack.AttachmentField{
                Title: "Environment",
                Value: environment,
		Short: false,
            },
            slack.AttachmentField{
                Title: "Deployed Version",
                Value: tag,
		Short: true,
            },
            slack.AttachmentField{
                Title: "Previous Version",
                Value: oldtag,
		Short: true,
            },
            slack.AttachmentField{
                Title: "Build Message",
                Value: tagMessage,
		Short: false,
            },
        },
    }

    channelID, timestamp, err := api.PostMessage(
        os.Getenv("SLACK_CHANNEL"),
        slack.MsgOptionAttachments(attachment),
        slack.MsgOptionAsUser(true), // Add this if you want that the bot would post message as a user, otherwise it will send response using the default slackbot
    )
    if err != nil {
        fmt.Printf("%s\n", err)
        return
    }
    fmt.Printf("Message successfully sent to channel %s at %s\n", channelID, timestamp)
}
