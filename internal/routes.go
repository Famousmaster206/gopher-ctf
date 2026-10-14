package internal

import (
	"github.com/a-h/templ"
	"github.com/gin-gonic/gin"
	"github.com/openlyfree/gopher-ctf/internal/handlers"
	"github.com/openlyfree/gopher-ctf/ui/pages"
)

func RegisterRoutes(router *gin.Engine) {
	staticGroup := router.Group("/static")
	if gin.IsDebugging() {
		staticGroup.Use(NoCache())
	}
	staticGroup.Static("/", "./ui/static")
	router.StaticFile("/favicon.png", "./public/favicon.png")
	router.Use(UserLoader())
	router.GET("/", func(c *gin.Context) { Render(c, 200, pages.Index()) })
	router.GET("/login", handlers.LoginPageHandler)
	router.GET("/register", handlers.RegisterPageHandler)
	router.GET("/account", handlers.AccountHandler)
	router.GET("/gimme-secret-candy-vault", handlers.AdminLoginPageHandler)
	router.GET("/challenges", handlers.ChallengeIndexHandler)
	router.POST("/login", handlers.LoginHandler)
	router.POST("/register", handlers.RegisterHandler)
	router.POST("/register/reroll", handlers.RerollUsernameHandler)
	router.POST("/logout", handlers.LogoutHandler)
	router.POST("/gimme-secret-candy-vault", handlers.AdminLoginHandler)
	challengeGroup := router.Group("/challenges")
	challengeGroup.GET("/:id", handlers.ChallengeIndividualHandler)
	challengeGroup.POST("/:id/submit", handlers.SubmitFlagHandler)
	admin := router.Group("/secret-candy-vault")
	admin.Use(AdminAuth())
	admin.GET("/", handlers.AdminIndexHandler)
	admin.GET("/players", handlers.AdminPlayersHandler)
	admin.POST("/challenge", handlers.CreateChallengeHandler)
	admin.POST("/reload-config", handlers.ConfigReloadHandler)
	challengeAdmin := router.Group("/admin")
	challengeAdmin.Use(AdminAuth())
	challengeAdmin.GET("/challenges", handlers.AdminChallengesHandler)
	challengeAdmin.GET("/challenges/new", handlers.AdminAddChallengeHandler)
	challengeAdmin.POST("/challenges/:id", handlers.UpdateChallengeHandler)
	challengeAdmin.POST("/accounts/:id/delete", handlers.DeleteAccountHandler)
	router.GET("/scoreboard", handlers.ScoreboardHandler)
}

func Render(c *gin.Context, status int, cmp templ.Component) {
	c.Status(status)
	_ = cmp.Render(c.Request.Context(), c.Writer)
}
