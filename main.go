package main

import (
	"log"
	"os" // Import os to read environment variables

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/openlyfree/gopher-ctf/internal"
	"github.com/openlyfree/gopher-ctf/internal/shared"
)

func main() {
	shared.InitConfig()
	shared.DB = shared.InitDB()
	router := gin.Default()
	router.Use(sessions.Sessions("gopher_session", cookie.NewStore([]byte(shared.Config.Key))))
	internal.RegisterRoutes(router)

	// Vercel dynamically injects a PORT environment variable at runtime
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080" // Fallback to your local port if running locally
	}

	log.Printf("Starting server on port %s", port)
	log.Fatal(router.Run(":" + port))
}