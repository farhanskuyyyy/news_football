package config

import "os"

type Config struct {
	Port               string
	DBHost             string
	DBPort             string
	DBUser             string
	DBPass             string
	DBName             string
	RedisAddr          string
	NewsAPIURL         string
	NewsAPIKey         string
	SportmonksBaseURL  string
	SportmonksAPIToken string
	RefreshToken       string
	CronEnabled        bool
	CronInterval       string
}

func Load() *Config {
	return &Config{
		Port:               getEnv("PORT", "8080"),
		DBHost:             getEnv("DB_HOST", "localhost"),
		DBPort:             getEnv("DB_PORT", "5432"),
		DBUser:             getEnv("DB_USER", "postgres"),
		DBPass:             getEnv("DB_PASS", "postgres"),
		DBName:             getEnv("DB_NAME", "newsdb"),
		RedisAddr:          getEnv("REDIS_ADDR", "localhost:6379"),
		NewsAPIURL:         getEnv("NEWS_API_URL", "https://newsapi.org/v2/everything?q=European%20football&language=en&sortBy=publishedAt"),
		NewsAPIKey:         getEnv("NEWS_API_KEY", ""),
		SportmonksBaseURL:  getEnv("SPORTMONKS_BASE_URL", "https://api.sportmonks.com/v3"),
		SportmonksAPIToken: getEnv("SPORTMONKS_API_TOKEN", ""),
		RefreshToken:       getEnv("REFRESH_TOKEN", ""),
		CronEnabled:        getEnv("SCRAPE_CRON_ENABLED", "true") == "true",
		CronInterval:       getEnv("SCRAPE_CRON_INTERVAL", "30m"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
