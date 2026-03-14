package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

// GeoResponse est la réponse de l'API de géocodage Open-Meteo.
type GeoResponse struct {
	Results []struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
		Name      string  `json:"name"`
	} `json:"results"`
}

// WeatherResponse est la réponse de l'API météo Open-Meteo.
type WeatherResponse struct {
	Current struct {
		Temperature float64 `json:"temperature_2m"`
		Humidity    int     `json:"relative_humidity_2m"`
		WindSpeed   float64 `json:"wind_speed_10m"`
		WeatherCode int     `json:"weather_code"`
		Visibility  float64 `json:"visibility"`
	} `json:"current"`
	CityName string // rempli après le géocodage
}

// messages contains humorous status messages per weather condition
// %d entries receive the temperature in degrees Celsius
var messages = map[string][]string{
	"thunderstorm": {
		"⛈️ Thunderstorm — weather pod stuck in CrashLoopBackOff",
		"⛈️ Lightning outside — Thunder King has respawned, this is his raid now",
		"⛈️ Thunder — someone forgot to kubectl rollout restart the sky",
		"⛈️ Thunderstorm — just Zeus rebooting his bare-metal cluster again",
	},
	"drizzle": {
		"🌦️ Drizzle — sky is on a rolling update, slight leakage expected",
		"🌦️ Light rain — 95% humidity, feels like Stormwind on a Monday",
	},
	"rain": {
		"🌧️ Rain — rm -rf /going-outside --force",
		"🌧️ It's raining — the cloud provider is finally leaking something useful",
		"🌧️ Rain — AFK farming indoors like a proper Hearthstone enjoyer",
		"🌧️ It's raining — docker run --rm coffee:latest --volume desk",
	},
	"heavy_rain": {
		"🌧️ Heavy rain — kubectl drain outside --ignore-daemonsets",
		"🌧️ Downpour — Doomhammer is literally raining on my parade",
		"🌧️ Downpour — even the Murlocs are sheltering, Mrgglll",
	},
	"snow": {
		"❄️ Snow — Icecrown Citadel has spawned in my street",
		"❄️ Snowing — kubectl taint nodes/* weather=blizzard:NoSchedule",
		"❄️ Snow — The Lich King sends his regards, plus 5 cm of powder",
		"❄️ Snowing — apt freeze universe, confirm? [y/N]",
	},
	"clear": {
		"☀️ Sunny — git push origin good-vibes",
		"☀️ Clear sky — UV rays hit harder than a /who Orgrimmar at peak hours",
		"☀️ Nice weather — sky has been in Running state all morning",
		"☀️ Sunny — sudo chmod 777 /patio, finally",
	},
	"clear_hot": {
		"🥵 %d°C — thermal throttling IRL, like a node with no CPU limits",
		"🥵 %d°C — sudo apt install ac-unit — E: Package not found",
		"🥵 %d°C — even the Firelands think it's a bit much",
	},
	"clear_cold": {
		"🥶 %d°C — kubectl get warmth — No resources found",
		"🥶 %d°C — Frostmourne hungers... and so do I, haven't left the bed",
		"🥶 %d°C — systemctl status heating — ● dead (failed)",
	},
	"clouds": {
		"☁️ Cloudy — sky is migrating to a multi-cloud architecture",
		"🌥️ Overcast — sun is in Pending state, no available nodes",
		"☁️ Cloudy — even Sylvanas doesn't know if it'll clear up",
		"🌤️ Cloudy — Cloudflare put the sun behind a WAF again",
	},
	"fog": {
		"🌫️ Foggy — zero visibility, just like my Kubernetes docs",
		"🌫️ Misty — The Mists of Pandaria have arrived",
		"🌫️ Foggy — dmesg: WARNING: zero visibility ahead, check your lore",
	},
	"wind": {
		"💨 Windy — gusts detected, replicaset /hairstyle scaled down to 0",
		"💨 Wind — Sylvanas has set things on fire for less",
		"💨 Gusts — the wind has been DDoSing my hood since this morning",
	},
	"unknown": {
		"🌡️ Unknown weather — 404 Weather Not Found, lore is unclear",
		"🤔 Undefined atmospheric behavior — kubectl describe sky timed out",
		"❓ Weather state unknown — like the release date of the next WoW patch",
	},
}

func randomStatus(key string) string {
	msgs, ok := messages[key]
	if !ok || len(msgs) == 0 {
		msgs = messages["unknown"]
	}
	return msgs[rand.Intn(len(msgs))]
}

var httpClient = &http.Client{Timeout: 10 * time.Second}

// geocodeCity résoud un nom de ville en coordonnées via Open-Meteo.
func geocodeCity(city string) (lat, lon float64, name string, err error) {
	endpoint := fmt.Sprintf(
		"https://geocoding-api.open-meteo.com/v1/search?name=%s&count=1&language=fr",
		url.QueryEscape(city),
	)
	resp, err := httpClient.Get(endpoint)
	if err != nil {
		return 0, 0, "", fmt.Errorf("géocodage: %w", err)
	}
	defer resp.Body.Close()

	var geo GeoResponse
	if err := json.NewDecoder(resp.Body).Decode(&geo); err != nil {
		return 0, 0, "", fmt.Errorf("décodage géocodage: %w", err)
	}
	if len(geo.Results) == 0 {
		return 0, 0, "", fmt.Errorf("ville introuvable: %q", city)
	}
	r := geo.Results[0]
	return r.Latitude, r.Longitude, r.Name, nil
}

// fetchWeather récupère la météo actuelle via Open-Meteo (pas de clé API requise).
func fetchWeather(lat, lon float64, cityName string) (*WeatherResponse, error) {
	endpoint := fmt.Sprintf(
		"https://api.open-meteo.com/v1/forecast?latitude=%f&longitude=%f"+
			"&current=temperature_2m,relative_humidity_2m,wind_speed_10m,weather_code,visibility"+
			"&wind_speed_unit=ms",
		lat, lon,
	)
	resp, err := httpClient.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("requête météo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API météo: HTTP %d", resp.StatusCode)
	}

	var weather WeatherResponse
	if err := json.NewDecoder(resp.Body).Decode(&weather); err != nil {
		return nil, fmt.Errorf("décodage météo: %w", err)
	}
	weather.CityName = cityName
	return &weather, nil
}

// conditionKey convertit un code météo WMO (Open-Meteo) en clé de message.
// Référence : https://open-meteo.com/en/docs#weathervariables
func conditionKey(wmoCode, temp int, wind float64) string {
	switch {
	case wmoCode == 0 || wmoCode == 1:
		// Ciel dégagé / principalement dégagé
		if temp > 35 {
			return "clear_hot"
		}
		if temp < -5 {
			return "clear_cold"
		}
		if wind > 10 {
			return "wind"
		}
		return "clear"
	case wmoCode == 2 || wmoCode == 3:
		// Partiellement / complètement nuageux
		if wind > 10 {
			return "wind"
		}
		return "clouds"
	case wmoCode == 45 || wmoCode == 48:
		// Brouillard
		return "fog"
	case wmoCode >= 51 && wmoCode <= 57:
		// Bruine (légère, modérée, dense, verglassante)
		return "drizzle"
	case wmoCode == 61 || wmoCode == 63 || wmoCode == 66 || wmoCode == 80 || wmoCode == 81:
		// Pluie légère à modérée, averse légère
		return "rain"
	case wmoCode == 65 || wmoCode == 67 || wmoCode == 82:
		// Pluie forte, averse violente
		return "heavy_rain"
	case wmoCode == 71 || wmoCode == 73 || wmoCode == 75 || wmoCode == 77 || wmoCode == 85 || wmoCode == 86:
		// Neige (légère, modérée, forte, grains, averses)
		return "snow"
	case wmoCode == 95 || wmoCode == 96 || wmoCode == 99:
		// Orage
		return "thunderstorm"
	}
	return "unknown"
}

func buildStatus(w *WeatherResponse) string {
	temp := int(w.Current.Temperature)
	wind := w.Current.WindSpeed
	key := conditionKey(w.Current.WeatherCode, temp, wind)
	if key == "clear_hot" || key == "clear_cold" {
		return fmt.Sprintf(randomStatus(key), temp)
	}
	return randomStatus(key)
}

// customStatusPayload est le corps de la requête PATCH /users/@me/settings.
type customStatusPayload struct {
	CustomStatus struct {
		Text      string  `json:"text"`
		EmojiID   *string `json:"emoji_id"`
		EmojiName *string `json:"emoji_name"`
		ExpiresAt *string `json:"expires_at"`
	} `json:"custom_status"`
}

// setPersonalStatus met à jour le custom status du compte Discord via l'API REST.
//
// ⚠️  ATTENTION : utiliser un token utilisateur dans un script automatisé
// est contraire aux Conditions Générales d'Utilisation de Discord.
// Vous l'utilisez sous votre propre responsabilité.
func setPersonalStatus(token, statusText string) error {
	var payload customStatusPayload
	payload.CustomStatus.Text = statusText

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("sérialisation payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPatch, "https://discord.com/api/v9/users/@me/settings", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("création requête: %w", err)
	}
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("envoi requête Discord: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API Discord: HTTP %d — %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func main() {
	token := os.Getenv("DISCORD_TOKEN")
	city := os.Getenv("CITY")
	intervalStr := os.Getenv("UPDATE_INTERVAL_MINUTES")

	if token == "" {
		log.Fatal("❌  DISCORD_TOKEN manquant — voir .env.example")
	}
	if city == "" {
		city = "Paris"
		log.Println("⚠️   CITY non configurée, Paris utilisée par défaut")
	}

	interval := 15 * time.Minute
	if intervalStr != "" {
		if n, err := strconv.Atoi(intervalStr); err == nil && n > 0 {
			interval = time.Duration(n) * time.Minute
		}
	}

	// Résolution des coordonnées au démarrage (pas besoin de refaire à chaque cycle)
	lat, lon, cityName, err := geocodeCity(city)
	if err != nil {
		log.Fatalf("❌  Géocodage échoué: %v", err)
	}
	log.Printf("✅  Démarré — %s (%.4f, %.4f) — mise à jour toutes les %v", cityName, lat, lon, interval)

	doUpdate := func() {
		weather, err := fetchWeather(lat, lon, cityName)
		if err != nil {
			log.Printf("⚠️   Erreur météo: %v", err)
			return
		}

		statusText := buildStatus(weather)
		if err := setPersonalStatus(token, statusText); err != nil {
			log.Printf("⚠️   Mise à jour status Discord: %v", err)
			return
		}

		log.Printf("🔄  [%s | %.1f°C | vent %.1f m/s | WMO %d] → %s",
			weather.CityName, weather.Current.Temperature, weather.Current.WindSpeed, weather.Current.WeatherCode, statusText)
	}

	// Première mise à jour immédiate au démarrage
	doUpdate()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	for {
		select {
		case <-ticker.C:
			doUpdate()
		case <-quit:
			log.Println("👋  Arrêt propre — status météo désactivé")
			return
		}
	}
}
