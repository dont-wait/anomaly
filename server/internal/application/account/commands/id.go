package commands

import "go.mongodb.org/mongo-driver/v2/bson"

func newID() string {
	return bson.NewObjectID().Hex()
}
