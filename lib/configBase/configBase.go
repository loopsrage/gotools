package configBase

import (
	"github.com/loopsrage/gotools/lib/clsr"
	"github.com/loopsrage/gotools/lib/controller"
	"github.com/loopsrage/gotools/lib/featureSettings"
	"github.com/loopsrage/gotools/lib/settings"
	"reflect"
	"sync/atomic"
	"time"
)

type Feature interface {
	Source() string
}

type FeatureController struct {
	*controller.Controller `json:"-"`
	*ConfigBase            `json:"-"`

	Feature Feature `json:"-"`
	source  *atomic.Value
}

type ConfigBase struct {
	*featureSettings.Config
	*FeatureController

	Enabled bool `bson:"Enabled" json:"Enabled"`
}

func NewConfigBase(to *FeatureController) *ConfigBase {
	fcfg := featureSettings.NewConfig()
	cfg := &ConfigBase{Config: fcfg, FeatureController: to}

	fcfg.AggregateAttributes(cfg)
	if to != nil && cfg.Feature != nil {
		fcfg.AggregateAttributes(cfg.Feature)
	}

	return cfg
}

func getStructName(i interface{}) string {
	t := reflect.TypeOf(i)

	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if t.Kind() == reflect.Struct {
		return t.Name()
	}

	return ""
}

func NewFeatureController(feat Feature) *FeatureController {
	ft := &FeatureController{Feature: feat}

	ft.ConfigBase = NewConfigBase(ft)
	ft.source = &atomic.Value{}
	ft.source.Store(getStructName(feat))
	ft.Config = featureSettings.NewConfig()

	ft.Controller = controller.NewController(clsr.NewCleanupComplete(nil, nil), time.Minute, true)

	err := settings.JsonUnmarshalSettingsTo(ft.Feature.Source(), feat)
	if err != nil {
		panic(err)
	}

	err = settings.JsonUnmarshalSettingsTo(ft.Feature.Source(), ft.ConfigBase)
	if err != nil {
		panic(err)
	}

	ft.AggregateAttributes(ft.ConfigBase)
	ft.AggregateAttributes(feat)

	return ft
}
