package main

import (
	"backend.im/api"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.Default()
	r.Use(cors.New(cors.Config{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"GET", "POST"},
		AllowHeaders: []string{"Content-Type", "Authorization"},
	}))

	r.POST("/api/createbackend", api.Runk8s)
	r.POST("/api/testbackend", api.Testk8s)
	r.Run(":8080")

}
