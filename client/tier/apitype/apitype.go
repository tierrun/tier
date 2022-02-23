package apitype

import (
	"time"
)

type UpdateCount struct {
	CounterID string
	Now       time.Time
	Org       string
	Feature   string
	Op        string
	P         int
	N         int
}

type UpdateResponse struct {
	Used  int `json:"used"`
	Limit int `json:"limit"`
}

type StripeOptions struct {
	PublishableKey string `json:"publishableKey"`
	AccountID      string `json:"accountID"`
	ClientSecret   string `json:"clientSecret"`
}

type PushResult struct {
	Succeeded []string `json:"succeeded,omitempty"`
	Failed    []string `json:"failed,omitempty"`
}

type Mode string

// Modes
var (
	ModeVolume    Mode = "volume"
	ModeGraduated Mode = "graduated"
)

const Inf = 1<<63 - 1

type Tier struct {
	Upto  int `json:"upto"`
	Price int `json:"price"`
	Base  int `json:"base"`
}

type Feature struct {
	Title string `json:"title,omitempty"`

	Mode      Mode    `json:"type,omitempty"`
	Aggregate string  `json:"-,omitempty"`
	Interval  string  `json:"reset,omitempty"`
	Tiers     []*Tier `json:"tiers"`
}

func (f Feature) Limit() int {
	if len(f.Tiers) == 0 {
		return 0
	}
	return f.Tiers[len(f.Tiers)-1].Upto
}

type Plans map[string]*Plan

type Plan struct {
	Title    string              `json:"title"`
	Features map[string]*Feature `json:"features"`
}

type Model struct {
	Plans Plans `json:"plans"`
}
