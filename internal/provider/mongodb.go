package provider

import (
	"context"
	"fmt"
	"github.com/surendratiwari3/paota/config"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"time"
)

// MongoDBProviderInterface defines the methods for interacting with MongoDB.
type MongoDBProviderInterface interface {
	Insert(collection string, document interface{}) error
	Find(collection string, filter interface{}) ([]bson.M, error)
	Update(collection string, filter interface{}, update interface{}) error
	Delete(collection string, filter interface{}) error
}

type mongoDBClient struct {
	client      *mongo.Client
	mongoConfig config.MongoDBConfig
}

// NewMongoDBClient initializes a new MongoDB client and returns an interface to it.
func NewMongoDBClient(mongoConfig config.MongoDBConfig) (MongoDBProviderInterface, error) {
	client := &mongoDBClient{}
	err := client.Connect(mongoConfig)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func (m *mongoDBClient) Connect(mongoConfig config.MongoDBConfig) error {
	m.mongoConfig = mongoConfig
	return m.connect()
}

func (m *mongoDBClient) connect() error {
	clientOptions := options.Client().
		ApplyURI(m.mongoConfig.URI).
		SetMaxPoolSize(m.mongoConfig.MaxPoolSize) // Set max pool size as needed

	client, err := mongo.Connect(context.TODO(), clientOptions)
	if err != nil {
		return fmt.Errorf("could not connect to MongoDB: %w", err)
	}

	m.client = client

	// Verify the connection
	if err := m.client.Ping(context.TODO(), nil); err != nil {
		return fmt.Errorf("could not ping MongoDB: %w", err)
	}
	return nil
}

func (m *mongoDBClient) Disconnect() error {
	return m.client.Disconnect(context.TODO())
}

func (m *mongoDBClient) reconnect() error {
	for i := 0; i < 5; i++ { // Attempt to reconnect 5 times
		if err := m.connect(); err == nil {
			return nil
		}
		time.Sleep(2 * time.Second) // Wait before retrying
	}
	return fmt.Errorf("failed to reconnect after multiple attempts")
}

func (m *mongoDBClient) Insert(collection string, document interface{}) error {
	if err := m.checkConnection(); err != nil {
		return err
	}
	coll := m.client.Database(m.mongoConfig.DbName).Collection(collection)
	_, err := coll.InsertOne(context.TODO(), document)
	return err
}

func (m *mongoDBClient) Find(collection string, filter interface{}) ([]bson.M, error) {
	if err := m.checkConnection(); err != nil {
		return nil, err
	}
	coll := m.client.Database(m.mongoConfig.DbName).Collection(collection)
	cursor, err := coll.Find(context.TODO(), filter)
	if err != nil {
		return nil, err
	}
	var results []bson.M
	if err := cursor.All(context.TODO(), &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (m *mongoDBClient) Update(collection string, filter interface{}, update interface{}) error {
	if err := m.checkConnection(); err != nil {
		return err
	}
	coll := m.client.Database(m.mongoConfig.DbName).Collection(collection)
	_, err := coll.UpdateOne(context.TODO(), filter, update)
	return err
}

func (m *mongoDBClient) Delete(collection string, filter interface{}) error {
	if err := m.checkConnection(); err != nil {
		return err
	}
	coll := m.client.Database(m.mongoConfig.DbName).Collection(collection)
	_, err := coll.DeleteOne(context.TODO(), filter)
	return err
}

func (m *mongoDBClient) checkConnection() error {
	if err := m.client.Ping(context.TODO(), nil); err != nil {
		// Attempt to reconnect if the connection is lost
		fmt.Println("Connection lost, attempting to reconnect...")
		return m.reconnect()
	}
	return nil
}
