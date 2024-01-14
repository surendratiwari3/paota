package config

import "go.mongodb.org/mongo-driver/mongo"

// MongoDBConfig ...
type MongoDBConfig struct {
	Client   *mongo.Client
	Database string
}
