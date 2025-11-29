package config

import "go.mongodb.org/mongo-driver/mongo"

// MongoDBConfig ...
type MongoDBConfig struct {
	Client      *mongo.Client
	URI         string
	MaxPoolSize uint64 // Maximum number of connections in the pool
	DbName      string
}
