package cli

import (
	"laraserve/internal/projectconfig"
	"net"
	"strings"
)

// Infer local development infrastructure only. Remote services and credentials
// are never replaced; workers/migrations remain deliberate project choices.
func automaticServices(root string, c *projectconfig.Config) {
	if c.Services.AutoDetect != nil && !*c.Services.AutoDetect {
		return
	}
	values, err := projectconfig.EnvironmentValues(root)
	if err != nil {
		return
	}
	local := func(host string) bool {
		return host == "" || host == "localhost" || host == "redis" || host == "mailpit" || net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback()
	}
	redisNeeded := values["CACHE_STORE"] == "redis" || values["CACHE_DRIVER"] == "redis" || values["SESSION_DRIVER"] == "redis" || values["QUEUE_CONNECTION"] == "redis"
	if redisNeeded && local(values["REDIS_HOST"]) {
		c.Services.Redis = true
	}
	if strings.EqualFold(values["MAIL_MAILER"], "smtp") && local(values["MAIL_HOST"]) && (values["MAIL_HOST"] == "mailpit" || values["MAIL_PORT"] == "1025") {
		c.Services.Mailpit = true
	}
}
