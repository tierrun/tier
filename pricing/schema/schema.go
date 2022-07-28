package schema

import (
	"encoding/json"
	"math"
)

const Inf = math.MaxInt64

type Mode string

const (
	ModeGraduated Mode = "graduated"
	ModeVolume    Mode = "volume"
)

type Interval string

const (
	IntervalDaily   Interval = "@daily"
	IntervalWeekly  Interval = "@weekly"
	IntervalMonthly Interval = "@monthly"
	IntervalYearly  Interval = "@yearly"
)

type Aggregate string

const (
	AggregateSum Aggregate = "sum"
	AggregateMin Aggregate = "min"
	AggregateMax Aggregate = "max"
)

type Model struct {
	Plans Plans `json:"plans"`
}

type Plans []*Plan

func (ps *Plans) UnmarshalJSON(b []byte) error {
	var v map[string]*Plan
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*ps = nil
	for id, p := range v {
		p.ID = id
		*ps = append(*ps, p)
	}
	return nil
}

func (ps Plans) MarshalJSON() ([]byte, error) {
	return json.Marshal(ps.AsMap())
}

func (ps Plans) AsMap() map[string]*Plan {
	m := make(map[string]*Plan)
	for _, p := range ps {
		m[p.ID] = p
	}
	return m
}

type Plan struct {
	ProviderID string   `json:"-"`
	ID         string   `json:"-"`
	Title      string   `json:"title,omitempty"`
	Features   Features `json:"features,omitempty"`
}

type Features []*Feature

func (fs *Features) UnmarshalJSON(b []byte) error {
	var v map[string]*Feature
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*fs = nil
	for k, fp := range v {
		fp.ID = k
		*fs = append(*fs, fp)
	}
	return nil
}

func (fs Features) MarshalJSON() ([]byte, error) {
	m := make(map[string]*Feature)
	for _, fp := range fs {
		m[fp.ID] = fp
	}
	return json.Marshal(m)
}

type Feature struct {
	ProviderID string    `json:"-"`
	ID         string    `json:"-"`
	Plan       string    `json:"-"`
	Base       int64     `json:"base,omitempty"`
	Mode       Mode      `json:"mode,omitempty"`
	Aggregate  Aggregate `json:"aggregate,omitempty"`
	Interval   Interval  `json:"interval,omitempty"`
	Currency   string    `json:"currency,omitempty"`
	Tiers      []Tier    `json:"tiers,omitempty"`
}

type Tier struct {
	Upto  int64 `json:"upto,omitempty"`
	Price int64 `json:"price,omitempty"`
	Base  int64 `json:"base,omitempty"`
}

type tierJSON struct {
	Upto  *int64 `json:"upto,omitempty"`
	Price int64  `json:"price,omitempty"`
	Base  int64  `json:"base,omitempty"`
}

func (t *Tier) UnmarshalJSON(b []byte) error {
	var v tierJSON
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*t = Tier{}
	t.Price = v.Price
	t.Base = v.Base
	if v.Upto == nil {
		t.Upto = Inf
	} else {
		t.Upto = *v.Upto
	}
	return nil
}

func (t *Tier) MarshalJSON() ([]byte, error) {
	v := tierJSON{
		Price: t.Price,
		Base:  t.Base,
	}
	if t.Upto != Inf {
		v.Upto = &t.Upto
	}
	return json.Marshal(v)
}

type FeaturePlan struct {
	PlanProviderID string
	PlanID         string
	*Feature
}

func Validate(p *Plan) error {
	return nil
}
