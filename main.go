package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	weatherURL = "https://api.open-meteo.com/v1/forecast" +
		"?current=temperature_2m,apparent_temperature,relative_humidity_2m,precipitation,weather_code,wind_speed_10m,wind_direction_10m,wind_gusts_10m,uv_index" +
		"&daily=weather_code,temperature_2m_max,temperature_2m_min,temperature_2m_mean,relative_humidity_2m_mean,dew_point_2m_mean,leaf_wetness_probability_mean,precipitation_sum,precipitation_probability_max,wind_speed_10m_max,wind_speed_10m_mean,wind_gusts_10m_max,wind_direction_10m_dominant,uv_index_max,sunrise,sunset" +
		"&timezone=Europe%2FMadrid&forecast_days=7"

	// shortcut: CAMS europeo publica polen a ~4 días vista y calidad del aire a ~5 días, los días sin datos muestran "—".
	airURL = "https://air-quality-api.open-meteo.com/v1/air-quality" +
		"?current=european_aqi,pm10,pm2_5,nitrogen_dioxide,ozone,dust" +
		"&hourly=european_aqi,pm10,dust,olive_pollen,grass_pollen,mugwort_pollen,ragweed_pollen,birch_pollen,alder_pollen" +
		"&timezone=Europe%2FMadrid&forecast_days=7"
)

type config struct {
	Latitude  *float64          `json:"latitude"`
	Longitude *float64          `json:"longitude"`
	Profile   map[string]string `json:"profile"`
}

func loadConfig(path string) (config, error) {
	var cfg config
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		lat, lon := 37.7710, -1.5002
		return config{Latitude: &lat, Longitude: &lon}, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	if cfg.Latitude == nil || cfg.Longitude == nil ||
		math.Abs(*cfg.Latitude) > 90 || math.Abs(*cfg.Longitude) > 180 {
		return cfg, fmt.Errorf("latitude y longitude son obligatorias y deben estar en [-90, 90] y [-180, 180]")
	}
	return cfg, nil
}

type WeatherResp struct {
	Current struct {
		Time          string  `json:"time"`
		Temp          float64 `json:"temperature_2m"`
		Apparent      float64 `json:"apparent_temperature"`
		Humidity      int     `json:"relative_humidity_2m"`
		Precipitation float64 `json:"precipitation"`
		WeatherCode   int     `json:"weather_code"`
		WindSpeed     float64 `json:"wind_speed_10m"`
		WindDir       int     `json:"wind_direction_10m"`
		WindGusts     float64 `json:"wind_gusts_10m"`
		UVIndex       float64 `json:"uv_index"`
	} `json:"current"`
	Daily struct {
		Time            []string  `json:"time"`
		Code            []int     `json:"weather_code"`
		TempMax         []float64 `json:"temperature_2m_max"`
		TempMin         []float64 `json:"temperature_2m_min"`
		TempMean        []float64 `json:"temperature_2m_mean"`
		HumidityMean    []float64 `json:"relative_humidity_2m_mean"`
		DewPointMean    []float64 `json:"dew_point_2m_mean"`
		LeafWetnessMean []float64 `json:"leaf_wetness_probability_mean"`
		PrecipSum       []float64 `json:"precipitation_sum"`
		PrecipProb      []int     `json:"precipitation_probability_max"`
		WindMax         []float64 `json:"wind_speed_10m_max"`
		WindMean        []float64 `json:"wind_speed_10m_mean"`
		GustsMax        []float64 `json:"wind_gusts_10m_max"`
		WindDir         []int     `json:"wind_direction_10m_dominant"`
		UVMax           []float64 `json:"uv_index_max"`
		Sunrise         []string  `json:"sunrise"`
		Sunset          []string  `json:"sunset"`
	} `json:"daily"`
}

type AirResp struct {
	Current struct {
		AQI   *float64 `json:"european_aqi"`
		PM10  *float64 `json:"pm10"`
		PM25  *float64 `json:"pm2_5"`
		NO2   *float64 `json:"nitrogen_dioxide"`
		Ozone *float64 `json:"ozone"`
		Dust  *float64 `json:"dust"`
	} `json:"current"`
	Hourly struct {
		Time    []string   `json:"time"`
		AQI     []*float64 `json:"european_aqi"`
		PM10    []*float64 `json:"pm10"`
		Dust    []*float64 `json:"dust"`
		Olive   []*float64 `json:"olive_pollen"`
		Grass   []*float64 `json:"grass_pollen"`
		Mugwort []*float64 `json:"mugwort_pollen"`
		Ragweed []*float64 `json:"ragweed_pollen"`
		Birch   []*float64 `json:"birch_pollen"`
		Alder   []*float64 `json:"alder_pollen"`
	} `json:"hourly"`
}

type allergenSpec struct {
	name     string
	moderate float64
	high     float64
	fmtStr   string
	peaks    map[string]float64
}

var noColor = os.Getenv("NO_COLOR") != ""

const (
	cReset  = "\x1b[0m"
	cBold   = "\x1b[1m"
	cDim    = "\x1b[2m"
	cCyan   = "\x1b[36m"
	cGreen  = "\x1b[32m"
	cYellow = "\x1b[33m"
	cRed    = "\x1b[31m"
	cBlue   = "\x1b[34m"
)

func style(code, s string) string {
	if noColor || code == "" {
		return s
	}
	return code + s + cReset
}

func fetchJSON(client *http.Client, url string, dst any) error {
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d en %s", resp.StatusCode, url)
	}
	return json.NewDecoder(resp.Body).Decode(dst)
}

func dailyMax(times []string, vals []*float64) map[string]float64 {
	out := make(map[string]float64)
	for i, t := range times {
		if i >= len(vals) || vals[i] == nil || len(t) < 10 {
			continue
		}
		day := t[:10]
		if cur, ok := out[day]; !ok || *vals[i] > cur {
			out[day] = *vals[i]
		}
	}
	return out
}

func wmoDesc(code int) string {
	switch code {
	case 0:
		return "Despejado"
	case 1:
		return "Mayormente despejado"
	case 2:
		return "Parcialmente nuboso"
	case 3:
		return "Nuboso"
	case 45, 48:
		return "Niebla"
	case 51, 53, 55, 56, 57:
		return "Llovizna"
	case 61, 80:
		return "Lluvia débil"
	case 63, 81:
		return "Lluvia moderada"
	case 65, 66, 67, 82:
		return "Lluvia fuerte"
	case 71, 73, 75, 77, 85, 86:
		return "Nieve"
	case 95, 96, 99:
		return "Tormenta"
	default:
		return fmt.Sprintf("Código %d", code)
	}
}

func compass(deg int) string {
	dirs := []string{"N", "NE", "E", "SE", "S", "SO", "O", "NO"}
	idx := int(math.Round(float64(((deg%360)+360)%360)/45.0)) % 8
	return dirs[idx]
}

func aqiLabel(aqi float64) (string, string) {
	switch {
	case aqi <= 20:
		return "Buena", cGreen
	case aqi <= 40:
		return "Razonable", cGreen
	case aqi <= 60:
		return "Moderada", cYellow
	case aqi <= 80:
		return "Mala", cRed
	case aqi <= 100:
		return "Muy mala", cRed
	default:
		return "Extrema", cRed
	}
}

func levelBadge(val, mod, high float64) string {
	switch {
	case val >= high:
		return style(cRed, "● Alto")
	case val >= mod:
		return style(cYellow, "● Moderado")
	default:
		return style(cGreen, "● Bajo")
	}
}

func linScale(v, inMin, inMax, outMin, outMax float64) float64 {
	r := math.Max(0, math.Min(1, (v-inMin)/(inMax-inMin)))
	return outMin + (outMax-outMin)*r
}

// shortcut: índice meteorológico AirAware (0-100), estima condiciones favorables al moho sin captador físico de esporas.
func moldPotential(rh, leafWet, precip, temp, windKmh, dewPoint float64) float64 {
	var hScore, pScore, tScore, wScore float64
	switch {
	case rh >= 80:
		hScore = linScale(rh, 80, 100, 80, 100)
	case rh >= 65:
		hScore = linScale(rh, 65, 80, 40, 80)
	case rh >= 50:
		hScore = linScale(rh, 50, 65, 0, 40)
	}
	switch {
	case precip >= 10:
		pScore = linScale(precip, 10, 20, 80, 100)
	case precip >= 2:
		pScore = linScale(precip, 2, 10, 40, 80)
	case precip > 0:
		pScore = linScale(precip, 0.1, 2, 10, 40)
	}
	switch {
	case temp >= 15 && temp <= 30:
		tScore = 100
	case temp >= 5 && temp < 15:
		tScore = linScale(temp, 5, 15, 0, 70)
	case temp > 30 && temp <= 40:
		tScore = linScale(temp, 30, 40, 100, 30)
	}
	windMS := windKmh / 3.6
	switch {
	case windMS < 2:
		wScore = 100
	case windMS < 5:
		wScore = linScale(windMS, 2, 5, 100, 50)
	case windMS < 10:
		wScore = linScale(windMS, 5, 10, 50, 0)
	}
	lwScore := math.Max(0, math.Min(100, leafWet))
	weighted := 0.30*hScore + 0.25*lwScore + 0.20*pScore + 0.15*tScore + 0.10*wScore

	mod := -3.0
	if dep := temp - dewPoint; dep <= 2 {
		mod = 5
	} else if dep <= 5 {
		mod = 2
	}
	return math.Max(0, math.Min(100, math.Round(weighted+mod)))
}

// shortcut: Salsola, Ciprés y Plátano no existen en CAMS/Open-Meteo, se avisa por calendario aerobiológico REAREMUR (Murcia).
func seasonalStatus(month time.Month, peakMonths, activeMonths []time.Month, window string) string {
	for _, m := range peakMonths {
		if month == m {
			return style(cRed, "● Pico habitual ("+window+")")
		}
	}
	for _, m := range activeMonths {
		if month == m {
			return style(cYellow, "● En temporada ("+window+")")
		}
	}
	return style(cDim, "○ Fuera de temp. ("+window+")")
}

func colorTemp(t float64) string {
	s := fmt.Sprintf("%4.1f°", t)
	switch {
	case t >= 30:
		return style(cRed, s)
	case t >= 22:
		return style(cYellow, s)
	case t < 10:
		return style(cCyan, s)
	default:
		return s
	}
}

var (
	weekdaysES = [...]string{"Dom", "Lun", "Mar", "Mié", "Jue", "Vie", "Sáb"}
	monthsES   = [...]string{"", "ene", "feb", "mar", "abr", "may", "jun", "jul", "ago", "sep", "oct", "nov", "dic"}
)

func formatDay(iso string, isToday bool) string {
	t, err := time.Parse("2006-01-02", iso)
	if err != nil {
		return iso
	}
	if isToday {
		return fmt.Sprintf("Hoy %02d %s", t.Day(), monthsES[t.Month()])
	}
	return fmt.Sprintf("%s %02d %s", weekdaysES[t.Weekday()], t.Day(), monthsES[t.Month()])
}

func shortDay(iso string) string {
	t, err := time.Parse("2006-01-02", iso)
	if err != nil {
		return iso
	}
	return fmt.Sprintf("%s %02d", weekdaysES[t.Weekday()], t.Day())
}

func formatClock(iso string) string {
	if idx := strings.IndexByte(iso, 'T'); idx != -1 && len(iso) >= idx+6 {
		return iso[idx+1 : idx+6]
	}
	return iso
}

func fmtPtr(v *float64, format string) string {
	if v == nil {
		return "—"
	}
	return fmt.Sprintf(format, *v)
}

func visibleLen(s string) int {
	n := 0
	inEsc := false
	for _, r := range s {
		if r == '\x1b' {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		n++
	}
	return n
}

func padR(s string, w int) string {
	d := w - visibleLen(s)
	if d <= 0 {
		return s
	}
	return s + strings.Repeat(" ", d)
}

func padL(s string, w int) string {
	d := w - visibleLen(s)
	if d <= 0 {
		return s
	}
	return strings.Repeat(" ", d) + s
}

func render(out io.Writer, w WeatherResp, a AirResp, profile map[string]string) {
	const colW = 45
	totalW := colW*2 + 1

	nowHeader := "TOTANA (Murcia)"
	if len(w.Current.Time) >= 10 {
		if t, err := time.Parse("2006-01-02", w.Current.Time[:10]); err == nil {
			nowHeader += fmt.Sprintf(" · %s %02d %s %s", weekdaysES[t.Weekday()], t.Day(), monthsES[t.Month()], formatClock(w.Current.Time))
		}
	}

	minToday, maxToday := 0.0, 0.0
	rainProbToday, rainSumToday, uvMaxToday := 0, 0.0, 0.0
	sunrise, sunset := "—", "—"
	todayISO := ""
	if len(w.Daily.Time) > 0 {
		todayISO = w.Daily.Time[0]
		minToday = w.Daily.TempMin[0]
		maxToday = w.Daily.TempMax[0]
		rainProbToday = w.Daily.PrecipProb[0]
		rainSumToday = w.Daily.PrecipSum[0]
		uvMaxToday = w.Daily.UVMax[0]
		sunrise = formatClock(w.Daily.Sunrise[0])
		sunset = formatClock(w.Daily.Sunset[0])
	}

	aqiPeaks := dailyMax(a.Hourly.Time, a.Hourly.AQI)

	topSummary := fmt.Sprintf("%s (sensación %.1f°C) · %s · Mín %.1f° / Máx %.1f°",
		style(cBold, fmt.Sprintf("%.1f°C", w.Current.Temp)),
		w.Current.Apparent,
		style(cCyan, wmoDesc(w.Current.WeatherCode)),
		minToday, maxToday,
	)

	aqiTitle := style(cBold, "CALIDAD DEL AIRE")
	if a.Current.AQI != nil {
		lbl, col := aqiLabel(*a.Current.AQI)
		aqiTitle += fmt.Sprintf(" (EAQI %.0f · %s)", *a.Current.AQI, style(col, lbl))
	}
	aqiPeakToday := "—"
	if v, ok := aqiPeaks[todayISO]; ok {
		lbl, col := aqiLabel(v)
		aqiPeakToday = style(col, fmt.Sprintf("%.0f (%s)", v, lbl))
	}

	left := []string{
		style(cBold, "METEOROLOGÍA HOY"),
		fmt.Sprintf("Viento    %.0f km/h %s (rachas %.0f km/h)", w.Current.WindSpeed, compass(w.Current.WindDir), w.Current.WindGusts),
		fmt.Sprintf("Lluvia    %d%% prob · %.1f mm (ahora %.1f)", rainProbToday, rainSumToday, w.Current.Precipitation),
		fmt.Sprintf("Humedad   %d%% · UV %.1f (máx %.1f)", w.Current.Humidity, w.Current.UVIndex, uvMaxToday),
		fmt.Sprintf("Sol       %s -> %s", sunrise, sunset),
	}
	right := []string{
		aqiTitle,
		fmt.Sprintf("PM2.5   %s μg/m³   PM10  %s μg/m³", padL(fmtPtr(a.Current.PM25, "%.1f"), 5), padL(fmtPtr(a.Current.PM10, "%.1f"), 5)),
		fmt.Sprintf("Polvo   %s μg/m³   O₃    %s μg/m³", padL(fmtPtr(a.Current.Dust, "%.1f"), 5), padL(fmtPtr(a.Current.Ozone, "%.1f"), 5)),
		fmt.Sprintf("NO₂     %s μg/m³", padL(fmtPtr(a.Current.NO2, "%.1f"), 5)),
		fmt.Sprintf("Pico EAQI hoy: %s", aqiPeakToday),
	}

	border := func(s string) string { return style(cDim, s) }
	fmt.Fprintf(out, "\n%s\n", border("╭"+strings.Repeat("─", totalW)+"╮"))
	fmt.Fprintf(out, "%s  %s%s\n", border("│"), padR(style(cBold+cCyan, nowHeader), totalW-2), border("│"))
	fmt.Fprintf(out, "%s  %s%s\n", border("│"), padR(topSummary, totalW-2), border("│"))
	fmt.Fprintf(out, "%s\n", border("├"+strings.Repeat("─", colW)+"┬"+strings.Repeat("─", colW)+"┤"))
	for i := range left {
		fmt.Fprintf(out, "%s  %s%s  %s%s\n",
			border("│"), padR(left[i], colW-2),
			border("│"), padR(right[i], colW-2),
			border("│"),
		)
	}
	fmt.Fprintf(out, "%s\n", border("╰"+strings.Repeat("─", colW)+"┴"+strings.Repeat("─", colW)+"╯"))

	moldPeaks := make(map[string]float64)
	for i, day := range w.Daily.Time {
		if i < len(w.Daily.HumidityMean) && i < len(w.Daily.LeafWetnessMean) && i < len(w.Daily.TempMean) && i < len(w.Daily.WindMean) && i < len(w.Daily.DewPointMean) {
			moldPeaks[day] = moldPotential(w.Daily.HumidityMean[i], w.Daily.LeafWetnessMean[i], w.Daily.PrecipSum[i], w.Daily.TempMean[i], w.Daily.WindMean[i], w.Daily.DewPointMean[i])
		}
	}

	nextDays := w.Daily.Time
	if len(nextDays) > 5 {
		nextDays = nextDays[:5]
	}

	specs := []allergenSpec{
		{"Gramíneas", 25, 50, "%.1f", dailyMax(a.Hourly.Time, a.Hourly.Grass)},
		{"Olivo", 50, 200, "%.1f", dailyMax(a.Hourly.Time, a.Hourly.Olive)},
		{"Artemisa", 10, 25, "%.1f", dailyMax(a.Hourly.Time, a.Hourly.Mugwort)},
		{"Moho", 25, 55, "%.0f", moldPeaks},
		{"Polvo/Calima", 25, 50, "%.1f", dailyMax(a.Hourly.Time, a.Hourly.Dust)},
		{"PM10", 20, 45, "%.1f", dailyMax(a.Hourly.Time, a.Hourly.PM10)},
	}
	extraSpecs := []allergenSpec{
		{"Ambrosía", 10, 25, "%.1f", dailyMax(a.Hourly.Time, a.Hourly.Ragweed)},
		{"Abedul", 30, 80, "%.1f", dailyMax(a.Hourly.Time, a.Hourly.Birch)},
		{"Aliso", 30, 80, "%.1f", dailyMax(a.Hourly.Time, a.Hourly.Alder)},
	}
	var quietExtras []string
	for _, ex := range extraSpecs {
		active := false
		for _, d := range nextDays {
			if ex.peaks[d] >= 1.0 {
				active = true
				break
			}
		}
		if active {
			specs = append(specs, ex)
		} else {
			quietExtras = append(quietExtras, ex.name)
		}
	}

	profileCell := func(note string) string {
		if len(profile) == 0 {
			return ""
		}
		return padR(note, 13) + "  "
	}
	fmt.Fprintf(out, "\n  %s %s\n", style(cBold, "POLEN Y PARTÍCULAS"), style(cDim, "(polen gr/m³, polvo/PM10 μg/m³, moho 0-100)"))
	hdr := fmt.Sprintf("  %s  %s%s  %s", padR("Alérgeno", 13), profileCell("Tu perfil"), padL("Hoy", 6), padR("Nivel", 11))
	for i := 1; i < len(nextDays); i++ {
		hdr += "  " + padL(shortDay(nextDays[i]), 7)
	}
	fmt.Fprintln(out, style(cDim, hdr))
	fmt.Fprintf(out, "  %s\n", border(strings.Repeat("─", utf8.RuneCountInString(hdr)-2)))

	for _, sp := range specs {
		todayVal, ok := sp.peaks[todayISO]
		todayStr := "—"
		badge := style(cDim, "—")
		if ok {
			todayStr = fmt.Sprintf(sp.fmtStr, todayVal)
			badge = levelBadge(todayVal, sp.moderate, sp.high)
		}
		row := fmt.Sprintf("  %s  %s%s  %s", padR(sp.name, 13), profileCell(style(cDim, profile[sp.name])), padL(todayStr, 6), padR(badge, 11))
		for i := 1; i < len(nextDays); i++ {
			cell := style(cDim, "—")
			if v, has := sp.peaks[nextDays[i]]; has {
				s := fmt.Sprintf(sp.fmtStr, v)
				switch {
				case v >= sp.high:
					cell = style(cRed, s)
				case v >= sp.moderate:
					cell = style(cYellow, s)
				default:
					cell = s
				}
			}
			row += "  " + padL(cell, 7)
		}
		fmt.Fprintln(out, row)
	}
	if len(quietExtras) > 0 {
		fmt.Fprintf(out, "  %s\n", style(cDim, "Otros CAMS sin presencia (<1 gr/m³): "+strings.Join(quietExtras, ", ")))
	}

	curMonth := time.Now().Month()
	if t, err := time.Parse("2006-01-02", todayISO); err == nil {
		curMonth = t.Month()
	}
	fmt.Fprintf(out, "\n  %s %s\n", style(cBold, "OTROS PÓLENES"), style(cDim, "(sin modelo diario CAMS · calendario Murcia REAREMUR)"))
	fmt.Fprintf(out, "  %s  %s%s\n", padR("Salsola", 13), profileCell(style(cDim, profile["Salsola"])),
		seasonalStatus(curMonth, []time.Month{5, 8, 9}, []time.Month{4, 6, 7, 10}, "abr-oct, picos may y ago-sep"))
	fmt.Fprintf(out, "  %s  %s%s\n", padR("Ciprés/Ariz.", 13), profileCell(style(cDim, profile["Ciprés/Ariz."])),
		seasonalStatus(curMonth, []time.Month{1, 2}, []time.Month{11, 12, 3}, "nov-mar, pico ene-feb"))
	fmt.Fprintf(out, "  %s  %s%s\n", padR("Plátano somb.", 13), profileCell(style(cDim, profile["Plátano somb."])),
		seasonalStatus(curMonth, []time.Month{3, 4}, nil, "mar-abr"))

	fmt.Fprintf(out, "\n  %s\n", style(cBold, "PRÓXIMOS 7 DÍAS"))
	dHdr := fmt.Sprintf("  %s  %s  %s  %s  %s  %s  %s",
		padR("Día", 11),
		padR("Estado", 21),
		padR("Mín / Máx", 15),
		padR("Lluvia", 14),
		padR("Viento (rachas)", 19),
		padL("UV", 4),
		padL("EAQI", 5),
	)
	fmt.Fprintln(out, style(cDim, dHdr))
	fmt.Fprintf(out, "  %s\n", border(strings.Repeat("─", utf8.RuneCountInString(dHdr)-2)))

	for i, day := range w.Daily.Time {
		rainStr := fmt.Sprintf("%2d%% · %4.1f mm", w.Daily.PrecipProb[i], w.Daily.PrecipSum[i])
		if w.Daily.PrecipSum[i] > 0 || w.Daily.PrecipProb[i] >= 40 {
			rainStr = style(cBlue, rainStr)
		}
		windStr := fmt.Sprintf("%2.0f (%2.0f) km/h %-2s", w.Daily.WindMax[i], w.Daily.GustsMax[i], compass(w.Daily.WindDir[i]))
		if w.Daily.GustsMax[i] >= 50 {
			windStr = style(cYellow, windStr)
		}
		aqiStr := style(cDim, "—")
		if v, ok := aqiPeaks[day]; ok {
			_, col := aqiLabel(v)
			aqiStr = style(col, fmt.Sprintf("%.0f", v))
		}
		temps := fmt.Sprintf("%s / %s", colorTemp(w.Daily.TempMin[i]), colorTemp(w.Daily.TempMax[i]))
		fmt.Fprintf(out, "  %s  %s  %s  %s  %s  %s  %s\n",
			padR(formatDay(day, i == 0), 11),
			padR(wmoDesc(w.Daily.Code[i]), 21),
			padR(temps, 15),
			padR(rainStr, 14),
			padR(windStr, 19),
			padL(fmt.Sprintf("%.1f", w.Daily.UVMax[i]), 4),
			padL(aqiStr, 5),
		)
	}
	fmt.Fprintln(out)
}

func main() {
	dir, err := os.UserConfigDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error localizando la configuración: %v\n", err)
		os.Exit(1)
	}
	cfg, err := loadConfig(filepath.Join(dir, "tiempo", "config.json"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error leyendo config.json: %v\n", err)
		os.Exit(1)
	}
	coords := fmt.Sprintf("&latitude=%g&longitude=%g", *cfg.Latitude, *cfg.Longitude)
	client := &http.Client{Timeout: 10 * time.Second}
	var (
		w    WeatherResp
		a    AirResp
		wErr error
		aErr error
		wg   sync.WaitGroup
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		wErr = fetchJSON(client, weatherURL+coords, &w)
	}()
	go func() {
		defer wg.Done()
		aErr = fetchJSON(client, airURL+coords, &a)
	}()
	wg.Wait()

	if wErr != nil {
		fmt.Fprintf(os.Stderr, "Error obteniendo meteorología: %v\n", wErr)
		os.Exit(1)
	}
	if aErr != nil {
		fmt.Fprintf(os.Stderr, "Aviso: sin datos de calidad del aire/alérgenos (%v)\n", aErr)
	}

	render(os.Stdout, w, a, cfg.Profile)
}
