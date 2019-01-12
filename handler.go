package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/nlopes/slack"
	"github.com/otoyo/garoon"
)

// interactionHandler handles interactive message response.
type interactionHandler struct {
	ownerSlackID                string
	slackClient                 *slack.Client
	garoonClient                *garoon.Client
	garoonExcludingFacilityCode string
	verificationToken           string
}

type AvailableTimes []garoon.AvailableTime

func (a AvailableTimes) Len() int {
	return len(a)
}

func (a AvailableTimes) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

func (a AvailableTimes) Less(i, j int) bool {
	if a[i].Start.DateTime.Equal(a[j].Start.DateTime) {
		return a[i].Facility.Code < a[j].Facility.Code
	}
	return a[i].Start.DateTime.Before(a[j].Start.DateTime)
}

func (h interactionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		log.Printf("[ERROR] Invalid method: %s", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	buf, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Printf("[ERROR] Failed to read request body: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	jsonStr, err := url.QueryUnescape(string(buf)[8:])
	if err != nil {
		log.Printf("[ERROR] Failed to unespace request body: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var message slack.AttachmentActionCallback
	if err := json.Unmarshal([]byte(jsonStr), &message); err != nil {
		log.Printf("[ERROR] Failed to decode json message from slack: %s", jsonStr)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Only accept message from slack with valid token
	if message.Token != h.verificationToken {
		log.Printf("[ERROR] Invalid token: %s", message.Token)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// Overwrite original message.
	message.OriginalMessage.ReplaceOriginal = true
	message.OriginalMessage.ResponseType = "in_channel"

	var text string
	action := message.Actions[0]
	switch action.Name {
	case actionSelectTarget:
		subject := strings.SplitN(action.SelectedOptions[0].Value, ",", 2)[1]
		text = fmt.Sprintf(":ok: %s was selected.\nPlease wait.", subject)
	case actionSelectTime:
		start, _ := time.Parse("2006-01-02T15:04:05-07:00", strings.Split(action.SelectedOptions[0].Value, ",")[1])
		text = fmt.Sprintf(":ok: %s was selected.\nPlease wait.", start.Format("2006-01-02 15:04"))
	case actionCancel:
		text = fmt.Sprintf("@%s canceled.", message.User.Name)
		return
	default:
		log.Printf("[ERROR] ]Invalid action was submitted: %s", action.Name)
		return
	}
	responseMessage(w, message.OriginalMessage, text, "")

	go h.asyncResponse(message)
	return
}

func (h interactionHandler) asyncResponse(message slack.AttachmentActionCallback) {
	if len(message.OriginalMessage.Attachments) == 0 {
		log.Printf("[ERROR] no attachments in actionSelectTarget")
		return
	}
	attachment := &message.OriginalMessage.Attachments[0]
	attachment.Title = ""
	attachment.Text = ""
	attachment.Fields = nil
	attachment.Actions = nil

	action := message.Actions[0]
	switch action.Name {
	case actionSelectTarget:
		eventID := strings.SplitN(action.SelectedOptions[0].Value, ",", 2)[0]
		ev, err := h.garoonClient.FindEvent(eventID)
		if err != nil {
			log.Printf("[ERROR] failed to find the event: %s", err)
			return
		}

		if err = h.setupAttachment(message.User.ID, ev, attachment); err != nil {
			log.Printf("[ERROR] failed to setup attachment: %s", err)
			return
		}
	case actionSelectTime:
		if message.User.ID != h.ownerSlackID {
			attachment.Title = ":x: You are not permitted."
		} else {
			s := strings.Split(action.SelectedOptions[0].Value, ",")

			if err := h.updateEvent(s[0], s[1], s[2], s[3]); err != nil {
				log.Printf("[ERROR] failed to update the event: %s", err)
				return
			}

			attachment.Title = "The schedule has been rescheduled! :white_check_mark:"
		}
	case actionCancel:
		attachment.Title = fmt.Sprintf("@%s canceled.", message.User.Name)
	default:
		log.Printf("[ERROR] ]Invalid action was submitted: %s", action.Name)
		return
	}

	params := slack.PostMessageParameters{
		Attachments: []slack.Attachment{
			*attachment,
		},
	}

	if _, _, err := h.slackClient.PostMessage(message.Channel.ID, "", params); err != nil {
		log.Printf("failed to post message: %s", err)
		return
	}
}

// responseMessage response to the original slackbutton enabled message.
// It removes button and replace it with message which indicate how bot will work
func responseMessage(w http.ResponseWriter, original slack.Message, text, value string) {
	if len(original.Attachments) == 0 {
		original.Attachments = append(original.Attachments, slack.Attachment{})
	}
	original.Attachments[0].Actions = []slack.AttachmentAction{} // empty buttons
	original.Attachments[0].Text = text

	w.Header().Add("Content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(&original)
}

func (h interactionHandler) setupAttachment(messageUserID string, ev *garoon.Event, attachment *slack.Attachment) error {
	if ev.IsAllDay || ev.IsStartOnly {
		attachment.Title = "No end MTG does not supported."
		return nil
	}

	if len(ev.Facilities) > 1 {
		attachment.Title = "Multiple MTG rooms does not supported."
		return nil
	}

	availableTimes, err := h.searchAvailableTimes(ev)
	if err != nil {
		log.Printf("[ERROR] failed to search availableTimes: %s", err)
		return err
	}

	if len(availableTimes) == 0 {
		attachment.Title = "Could not find a date to reschedule."
		return nil
	}

	const layoutForRead = "2006-01-02 15:04"
	const layoutForValue = "2006-01-02T15:04:05-07:00"
	var text string
	var options []slack.AttachmentActionOption

	for _, t := range availableTimes {
		start := t.Start.DateTime
		end := t.End.DateTime

		text = fmt.Sprintf("%s%s %s\n", text, start.Format(layoutForRead), t.Facility.Name)
		options = append(options, slack.AttachmentActionOption{
			Text:  fmt.Sprintf("%s %s", start.Format(layoutForRead), t.Facility.Name),
			Value: fmt.Sprintf("%s,%s,%s,%s", ev.ID, start.Format(layoutForValue), end.Format(layoutForValue), t.Facility.ID),
		})
	}
	attachment.Text = text

	if messageUserID != h.ownerSlackID {
		attachment.Title = fmt.Sprintf("%d schedules found.", len(availableTimes))
		return nil
	}

	attachment.Title = "Which shedule would you like?"
	attachment.Actions = []slack.AttachmentAction{
		{
			Name:    actionSelectTime,
			Type:    "select",
			Options: options,
		},
		{
			Name:  actionCancel,
			Text:  "Cancel",
			Type:  "button",
			Style: "danger",
		},
	}

	return nil
}

func (h interactionHandler) searchAvailableTimes(ev *garoon.Event) ([]garoon.AvailableTime, error) {
	facilities, err := h.getFacilitiesFromOwnFacilityGroup(ev.Facilities)
	if err != nil {
		log.Printf("[ERROR] failed to get facilities: %s", err)
		return nil, err
	}

	params, err := h.buildAvailableTimeParameters(ev, facilities)
	if err != nil {
		log.Printf("[ERROR] failed to build AvailableTimeParamter: %s", err)
		return nil, err
	}

	var availableTimes AvailableTimes
	for _, param := range params {
		pager, err := h.garoonClient.SearchAvailableTimes(&param)
		if err != nil {
			log.Printf("[ERROR] failed to get AvailableTimes: %s", err)
			return nil, err
		}
		availableTimes = append(availableTimes, pager.AvailableTimes...)
	}

	// Sort by datetime asc
	sort.Sort(availableTimes)

	return availableTimes, nil
}

func (h interactionHandler) buildAvailableTimeParameters(ev *garoon.Event, facilities []garoon.Facility) ([]garoon.AvailableTimeParameter, error) {
	ranges, err := h.buildTimeRanges(ev)
	if err != nil {
		return nil, err
	}

	interval := ev.End.DateTime.Sub(ev.Start.DateTime)

	var params []garoon.AvailableTimeParameter
	for _, r := range ranges {
		param := garoon.AvailableTimeParameter{
			TimeRanges:              []garoon.DateTimePeriod{r},
			TimeInterval:            fmt.Sprintf("%2.0f", interval.Minutes()),
			Attendees:               ev.Attendees,
			Facilities:              facilities,
			FacilitySearchCondition: "OR",
		}
		params = append(params, param)
	}

	return params, nil
}

func (h interactionHandler) buildTimeRanges(ev *garoon.Event) ([]garoon.DateTimePeriod, error) {
	periods := []garoon.DateTimePeriod{}

	end := ev.End.DateTime
	for i := 0; i <= 7; i++ {
		d := end.AddDate(0, 0, i)
		if d.Weekday() == 0 || d.Weekday() == 6 {
			continue
		}

		startHour := 10
		if i == 0 {
			hour := time.Now().Hour()
			if hour >= 19 {
				continue
			} else if hour > 10 {
				startHour = hour
			}
		}

		periods = append(periods, garoon.DateTimePeriod{
			Start: time.Date(d.Year(), d.Month(), d.Day(), startHour, 0, 0, 0, time.Local),
			End:   time.Date(d.Year(), d.Month(), d.Day(), 19, 0, 0, 0, time.Local),
		})
	}

	return periods, nil
}

func (h interactionHandler) getFacilitiesFromOwnFacilityGroup(facilities []garoon.Facility) ([]garoon.Facility, error) {
	if len(facilities) == 0 {
		return nil, nil
	}

	v := url.Values{}
	v.Add("name", facilities[0].Name)

	pager, err := h.garoonClient.GetFacilities(v)
	if err != nil {
		return nil, err
	}

	if len(pager.Facilities) == 0 {
		return nil, fmt.Errorf("target facilities not found.")
	}

	v = url.Values{}
	facilityGroupID := pager.Facilities[0].FacilityGroup
	pager, err = h.garoonClient.GetFacilitiesByFacilityGroup(facilityGroupID, v)
	if err != nil {
		return nil, fmt.Errorf("facilities in the facility group not found.")
	}

	// Exclude facilities
	codes := strings.Split(h.garoonExcludingFacilityCode, ",")
	var filteredFacilities []garoon.Facility
	for _, f := range pager.Facilities {
		isIncluded := false
		for _, code := range codes {
			if f.Code == code {
				isIncluded = true
			}
		}
		if !isIncluded {
			filteredFacilities = append(filteredFacilities, f)
		}
	}

	return filteredFacilities, nil
}

func (h interactionHandler) updateEvent(eventID, start, end, facilityID string) error {
	ev, err := h.garoonClient.FindEvent(eventID)
	if err != nil {
		return fmt.Errorf("failed to find the event.")
	}

	const layout = "2006-01-02T15:04:05-07:00"
	ev.Start.DateTime, _ = time.Parse(layout, start)
	ev.End.DateTime, _ = time.Parse(layout, end)
	ev.Facilities = []garoon.Facility{
		garoon.Facility{
			ID: facilityID,
		},
	}

	_, err = h.garoonClient.UpdateEvent(ev)
	if err != nil {
		return fmt.Errorf("failed to update the event.")
	}

	return nil
}
