package main

import (
	"log"
	"net/http"
	"os"

	"github.com/kelseyhightower/envconfig"
	"github.com/nlopes/slack"
	"github.com/otoyo/garoon"
)

// https://api.slack.com/slack-apps
// https://api.slack.com/internal-integrations
type envConfig struct {
	// Port is server port to be listened.
	Port string `envconfig:"PORT" default:"3000"`

	// SlackBotToken is bot user token to access to slack API.
	SlackBotToken string `envconfig:"SLACK_BOT_TOKEN" required:"true"`

	// SlackVerificationToken is used to validate interactive messages from slack.
	SlackVerificationToken string `envconfig:"SLACK_VERIFICATION_TOKEN" required:"true"`

	// SlackBotID is bot user ID.
	SlackBotID string `envconfig:"SLACK_BOT_ID" required:"true"`

	// SlackChannelID is slack channel ID.
	// Bot responses to the mention in this channel.
	SlackChannelID string `envconfig:"SLACK_CHANNEL_ID"`

	// SlackUserID is your slack user ID.
	SlackUserID string `envconfig:"SLACK_USER_ID" required:"true"`

	// GaroonURL is URL for Garoon REST API
	GaroonSubdomain string `envconfig:"GAROON_SUBDOMAIN" required:"true"`

	// GaroonUser is login user for Garoon
	GaroonUser string `envconfig:"GAROON_USER" required:"true"`

	// GaroonPassword is login password for Garoon
	GaroonPassword string `envconfig:"GAROON_PASSWORD" required:"true"`
}

func main() {
	os.Exit(_main(os.Args[1:]))
}

func _main(args []string) int {
	var env envConfig
	if err := envconfig.Process("", &env); err != nil {
		log.Printf("[ERROR] Failed to process env var: %s", err)
		return 1
	}

	garoonClient, err := garoon.NewClient(env.GaroonSubdomain, env.GaroonUser, env.GaroonPassword)
	if err != nil {
		log.Printf("[ERROR] Failed to create Garoon client: %s", err)
		return 1
	}

	// Listening slack event and response
	log.Printf("[INFO] Start slack event listening")
	slackClient := slack.New(env.SlackBotToken)
	slackListener := &SlackListener{
		client:       slackClient,
		botID:        env.SlackBotID,
		channelID:    env.SlackChannelID,
		ownerID:      env.SlackUserID,
		garoonClient: garoonClient,
	}
	go slackListener.ListenAndResponse()

	// Register handler to receive interactive message
	// responses from slack (kicked by user action)
	http.Handle("/interaction", interactionHandler{
		verificationToken: env.SlackVerificationToken,
	})

	log.Printf("[INFO] Server listening on :%s", env.Port)
	if err := http.ListenAndServe(":"+env.Port, nil); err != nil {
		log.Printf("[ERROR] %s", err)
		return 1
	}

	return 0
}
