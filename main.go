package main

import (
	"log"
	"task/queue/bootstrap/config"
	"task/queue/bootstrap/server"

	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("env file not found")
	}

	conf, err := config.Init()
	if err != nil {
		panic(err)
	}
	defer conf.DB.Close()

	if err := server.Listen(conf.Port, conf.DB); err != nil {
		log.Fatal(err)
	}
}
