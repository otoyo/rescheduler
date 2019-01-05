package main

import (
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/nlopes/slack"
	"github.com/otoyo/garoon"
)

const (
	// action is used for slack attament action.
	actionSelectTarget = "selectTarget"
	actionSelectTime   = "selectTime"
	actionCancel       = "cancel"
)

type SlackListener struct {
	client       *slack.Client
	botID        string
	channelID    string
	ownerID      string
	garoonClient *garoon.Client
}

// LstenAndResponse listens slack events and response
// particular messages. It replies by slack message button.
func (s *SlackListener) ListenAndResponse() {
	rtm := s.client.NewRTM()

	// Start listening slack events
	go rtm.ManageConnection()

	// Handle slack events
	for msg := range rtm.IncomingEvents {
		switch ev := msg.Data.(type) {
		case *slack.MessageEvent:
			if err := s.handleMessageEvent(ev); err != nil {
				log.Printf("[ERROR] Failed to handle message: %s", err)
			}
		}
	}
}

// handleMesageEvent handles message events.
func (s *SlackListener) handleMessageEvent(ev *slack.MessageEvent) error {
	// Only response mention to bot. Ignore else.
	if !strings.HasPrefix(ev.Msg.Text, fmt.Sprintf("<@%s> ", s.botID)) {
		return nil
	}

	// If channelID is set, bot only responses in specific channel. Ignore else.
	if s.channelID != "" && ev.Channel != s.channelID {
		return nil
	}

	var attachment *slack.Attachment

	// Parse message
	m := strings.Split(strings.TrimSpace(ev.Msg.Text), " ")[1:]

	if len(m) != 2 || m[0] != "search" {
		attachment = &slack.Attachment{
			Text:  "Would you mind ordering like `@rescheduler search Foo`?",
			Color: "#00bfff",
		}
	} else {
		// Search MTG schedules by keyword
		pager, err := s.searchSchedules(m[1])
		if err != nil {
			return err
		}

		attachment, err = s.setupAttachment(pager)
		if err != nil {
			return err
		}
	}

	params := slack.PostMessageParameters{
		Attachments: []slack.Attachment{
			*attachment,
		},
	}

	if _, _, err := s.client.PostMessage(ev.Channel, "", params); err != nil {
		return fmt.Errorf("failed to post message: %s", err)
	}

	return nil
}

func (s *SlackListener) searchSchedules(keyword string) (*garoon.EventPager, error) {
	const layout = "2006-01-02T15:04:05-07:00"

	v := url.Values{}
	v.Add("keyword", keyword)
	v.Add("excludeFromSearch", "company,notes,comments")
	v.Add("rangeStart", time.Now().Format(layout))
	v.Add("orderBy", "createdAt asc")

	pager, err := s.garoonClient.SearchEvents(v)
	if err != nil {
		return nil, fmt.Errorf("failed to search schedules: %s", err)
	}

	return pager, nil
}

func (s *SlackListener) setupAttachment(pager *garoon.EventPager) (*slack.Attachment, error) {
	const layout = "2006-01-02 15:04"

	var options []slack.AttachmentActionOption
	for _, ev := range pager.Events {
		options = append(options, slack.AttachmentActionOption{
			Text:  fmt.Sprintf("%s %s", ev.Start.DateTime.Format(layout), ev.Subject),
			Value: ev.ID,
		})
	}

	// value is passed to message handler when request is approved.
	attachment := &slack.Attachment{
		Title:      "Which schedule do you intend? :calendar:",
		Color:      "#32cd32",
		CallbackID: "target",
		Actions: []slack.AttachmentAction{
			{
				Name:    actionSelectTarget,
				Type:    "select",
				Options: options,
			},
			{
				Name:  actionCancel,
				Text:  "Cancel",
				Type:  "button",
				Style: "danger",
			},
		},
	}

	return attachment, nil
}
