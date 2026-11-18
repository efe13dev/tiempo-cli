package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func ptr(v float64) *float64 { return &v }

func TestLoadConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	example, err := loadConfig("config.example.json")
	if err != nil {
		t.Fatal(err)
	}
	if *cfg.Latitude != *example.Latitude || *cfg.Longitude != *example.Longitude || len(cfg.Profile) != 0 || len(example.Profile) != 0 {
		t.Fatal("missing config should use the public example location without a profile")
	}

	for _, tc := range []struct {
		json   string
		wantOK bool
	}{
		{`{"latitude":0,"longitude":0,"profile":{"Olivo":"nota de prueba"}}`, true},
		{`{"latitude":0}`, false},
		{`{"latitude":91,"longitude":0}`, false},
		{`{"latitude":0,"longitude":181}`, false},
		{`{"latitude":0,"longitude":null}`, false},
		{`{`, false},
	} {
		if err := os.WriteFile(path, []byte(tc.json), 0600); err != nil {
			t.Fatal(err)
		}
		cfg, err := loadConfig(path)
		if (err == nil) != tc.wantOK {
			t.Fatalf("loadConfig(%s): unexpected error result: %v", tc.json, err)
		}
		if tc.wantOK && (*cfg.Latitude != 0 || *cfg.Longitude != 0 || cfg.Profile["Olivo"] != "nota de prueba") {
			t.Fatal("local coordinates and profile should override the public defaults")
		}
	}
}

func TestDailyMaxAndRender(t *testing.T) {
	previousNoColor := noColor
	t.Cleanup(func() { noColor = previousNoColor })
	noColor = true
	times := []string{"2026-11-16T00:00", "2026-11-16T12:00", "2026-11-17T08:00"}
	vals := []*float64{ptr(12.5), ptr(64.0), nil}
	peaks := dailyMax(times, vals)

	if peaks["2026-11-16"] != 64.0 {
		t.Fatalf("expected peak 64.0, got %v", peaks["2026-11-16"])
	}
	if _, ok := peaks["2026-11-17"]; ok {
		t.Fatalf("expected nil day to be absent from peaks")
	}
	if compass(270) != "O" || compass(355) != "N" || compass(225) != "SO" {
		t.Fatalf("unexpected compass conversion: 270=%s 355=%s 225=%s", compass(270), compass(355), compass(225))
	}

	var w WeatherResp
	w.Current.Time = "2026-11-16T09:30"
	w.Current.Temp = 18.4
	w.Current.Apparent = 17.2
	w.Current.Humidity = 55
	w.Current.WeatherCode = 0
	w.Current.WindSpeed = 12
	w.Current.WindDir = 90
	w.Current.WindGusts = 22
	w.Daily.Time = []string{"2026-11-16", "2026-11-17"}
	w.Daily.Code = []int{0, 61}
	w.Daily.TempMin = []float64{10.0, 12.0}
	w.Daily.TempMax = []float64{23.0, 21.5}
	w.Daily.TempMean = []float64{15.4, 20.0}
	w.Daily.HumidityMean = []float64{62, 85}
	w.Daily.DewPointMean = []float64{7.5, 18.5}
	w.Daily.LeafWetnessMean = []float64{29, 80}
	w.Daily.PrecipProb = []int{0, 65}
	w.Daily.PrecipSum = []float64{0, 3.4}
	w.Daily.WindMax = []float64{15, 28}
	w.Daily.WindMean = []float64{8.0, 6.0}
	w.Daily.GustsMax = []float64{26, 52}
	w.Daily.WindDir = []int{90, 225}
	w.Daily.UVMax = []float64{3.6, 2.8}
	w.Daily.Sunrise = []string{"2026-11-16T07:47", "2026-11-17T07:48"}
	w.Daily.Sunset = []string{"2026-11-16T17:54", "2026-11-17T17:53"}

	var a AirResp
	a.Current.AQI = ptr(18)
	a.Current.PM25 = ptr(4.2)
	a.Current.PM10 = ptr(9.1)
	a.Hourly.Time = times
	a.Hourly.AQI = []*float64{ptr(18), ptr(24), nil}
	a.Hourly.Olive = []*float64{ptr(10), ptr(64), nil}

	var buf bytes.Buffer
	render(&buf, w, a, nil)
	out := buf.String()
	if strings.Contains(out, "Tu perfil") {
		t.Fatal("public output should not show the profile column")
	}

	if low, high := moldPotential(45, 5, 0, 20, 25, 5), moldPotential(85, 80, 5, 20, 5, 19); !(low < 25 && high >= 55) {
		t.Fatalf("unexpected moldPotential scores: dry=%v wet=%v", low, high)
	}

	for _, want := range []string{"TOTANA (Murcia)", "EAQI 18 · Buena", "Olivo", "Moho", "Otros CAMS sin presencia (<1 gr/m³): Ambrosía, Abedul, Aliso", "Salsola", "Ciprés/Ariz.", "● En temporada", "64.0", "● Moderado", "Lluvia débil", "3.4 mm"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output:\n%s", want, out)
		}
	}

	a.Hourly.Ragweed = []*float64{ptr(0), ptr(14.2), nil}
	buf.Reset()
	render(&buf, w, a, map[string]string{"Ambrosía": "nota de prueba", "Salsola": "nota estacional"})
	out = buf.String()
	if !strings.Contains(out, "Ambrosía") || !strings.Contains(out, "14.2") {
		t.Fatalf("expected active Ambrosía row when >=1.0:\n%s", buf.String())
	}
	for _, want := range []string{"Tu perfil", "nota de prueba", "nota estacional"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in private output", want)
		}
	}
}
