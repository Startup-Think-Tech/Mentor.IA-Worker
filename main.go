package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func main() {
	router := gin.Default()

	//func recebe um parametro c do tipo *gin.Context
	//c contem o endereço de memoria de um gin.Context
	//Context é um tipo vindo do pacote gin
	
	router.GET("/hello", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "hello world",
		})	
	})

	router.Run(":8080")
}