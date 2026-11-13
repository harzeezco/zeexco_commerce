package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/harzeezco/zeexco_commerce/internal/config"
	"github.com/harzeezco/zeexco_commerce/internal/features/whatsapp/cloudapi"
)

const testMessage = "Hello from Zeexco"

func main() {
	cfg := config.Load()
	recipient := os.Getenv("WHATSAPP_TEST_RECIPIENT")

	client, err := cloudapi.NewClient(cloudapi.ClientConfig{
		AccessToken:   cfg.WhatsAppAccessToken,
		PhoneNumberID: cfg.WhatsAppPhoneNumberID,
		APIVersion:    cfg.MetaGraphAPIVersion,
		Logger:        log.Default(),
	})
	if err != nil {
		log.Fatalf("initialize WhatsApp client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if _, err := client.SendTextMessage(ctx, recipient, testMessage); err != nil {
		log.Fatalf("send WhatsApp test message: %v", err)
	}

	log.Printf("whatsapp test message outcome=success")
}
