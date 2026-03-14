package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
)

// WeatherResponse représente la réponse de l'API OpenWeatherMap
type WeatherResponse struct {
	Weather []struct {
		ID          int    `json:"id"`
		Main        string `json:"main"`
		Description string `json:"description"`
	} `json:"weather"`
	Main struct {
		Temp      float64 `json:"temp"`
		FeelsLike float64 `json:"feels_like"`
	} `json:"main"`
	Wind struct {
		Speed float64 `json:"speed"`
	} `json:"wind"`
	Name string `json:"name"`
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

func fetchWeather(apiKey, city string) (*WeatherResponse, error) {
	endpoint := fmt.Sprintf(
		"https://api.openweathermap.org/data/2.5/weather?q=%s&appid=%s&units=metric&lang=fr",
		url.QueryEscape(city), apiKey,
	)

	resp, err := httpClient.Get(endpoint) //nolint:noctx
	if err != nil {
		return nil, fmt.Errorf("requête météo échouée: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API météo: HTTP %d pour la ville %q", resp.StatusCode, city)
	}

	var weather WeatherResponse
	if err := json.NewDecoder(resp.Body).Decode(&weather); err != nil {
		return nil, fmt.Errorf("décodage météo: %w", err)
	}

	return &weather, nil
}

func buildStatus(w *WeatherResponse) string {
	if len(w.Weather) == 0 {
		return randomStatus("unknown")
	}

	code := w.Weather[0].ID
	temp := int(w.Main.Temp)
	wind := w.Wind.Speed

	switch {
	case code >= 200 && code < 300:
		return randomStatus("thunderstorm")

	case code >= 300 && code < 400:
		return randomStatus("drizzle")

	case code >= 500 && code < 600:
		if code >= 502 {
			return randomStatus("heavy_rain")
		}
		return randomStatus("rain")

	case code >= 600 && code < 700:
		return randomStatus("snow")

	case code >= 700 && code < 800:
		return randomStatus("fog")

	case code == 800:
		// Les extrêmes thermiques priment sur le vent
		if temp > 35 {
			return fmt.Sprintf(randomStatus("clear_hot"), temp)
		}
		if temp < -5 {
			return fmt.Sprintf(randomStatus("clear_cold"), temp)
		}
		if wind > 10 {
			return randomStatus("wind")
		}
		return randomStatus("clear")

	case code >= 801 && code < 900:
		if wind > 10 {
			return randomStatus("wind")
		}
		return randomStatus("clouds")
	}

	return randomStatus("unknown")
}

// setPersonalStatus met à jour le statut personnalisé du compte Discord via Gateway opcode 3.
//
// ⚠️  ATTENTION : utiliser un token utilisateur dans un script automatisé
// est contraire aux Conditions Générales d'Utilisation de Discord.
// Vous l'utilisez sous votre propre responsabilité.
func setPersonalStatus(dg *discordgo.Session, statusText string) error {
	return dg.UpdateStatusComplex(discordgo.UpdateStatusData{
		Status: "online",
		Activities: []*discordgo.Activity{
			{
				Name:  "Custom Status",
				Type:  discordgo.ActivityTypeCustom,
				State: statusText,
			},
		},
	})
}

func main() {
	token := os.Getenv("DISCORD_TOKEN")
	apiKey := os.Getenv("OPENWEATHER_API_KEY")
	city := os.Getenv("CITY")
	intervalStr := os.Getenv("UPDATE_INTERVAL_MINUTES")

	if token == "" {
		log.Fatal("❌  DISCORD_TOKEN manquant — voir .env.example")
	}
	if apiKey == "" {
		log.Fatal("❌  OPENWEATHER_API_KEY manquant — voir .env.example")
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

	// Connexion Discord avec token utilisateur (pas de préfixe "Bot ")
	dg, err := discordgo.New(token)
	if err != nil {
		log.Fatalf("❌  Création de la session Discord: %v", err)
	}

	// Aucun intent bot nécessaire — on envoie uniquement des mises à jour de présence
	dg.Identify.Intents = 0

	doUpdate := func() {
		weather, err := fetchWeather(apiKey, city)
		if err != nil {
			log.Printf("⚠️   Erreur météo: %v", err)
			return
		}

		statusText := buildStatus(weather)
		if err := setPersonalStatus(dg, statusText); err != nil {
			log.Printf("⚠️   Mise à jour status Discord: %v", err)
			return
		}

		log.Printf("🔄  [%s | %.0f°C | vent %.1f m/s] → %s", weather.Name, weather.Main.Temp, weather.Wind.Speed, statusText)
	}

	// Déclencher une mise à jour dès que le gateway envoie READY, et à chaque reconnexion.
	// Le status de présence est réinitialisé par Discord à chaque reconnexion WebSocket,
	// il est donc nécessaire de le renvoyer à chaque fois.
	dg.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Printf("✅  Gateway READY — connecté en tant que %s (%s)", r.User.Username, r.User.ID)
		// Petit délai pour laisser la session se stabiliser avant d'envoyer l'opcode 3
		time.Sleep(500 * time.Millisecond)
		doUpdate()
	})

	if err := dg.Open(); err != nil {
		log.Fatalf("❌  Connexion au Gateway Discord: %v", err)
	}
	defer dg.Close()

	log.Printf("⏳  En attente du Gateway READY... (intervalle de mise à jour: %v, ville: %s)", interval, city)

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
