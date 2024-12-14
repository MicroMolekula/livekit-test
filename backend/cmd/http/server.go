package main

import (
	"backend/internal/controllers"
	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.Default()
	r.POST("/api/token", controllers.GetJoinToken)
	r.Run("localhost:8080") // listen and serve on 0.0.0.0:8080
}
