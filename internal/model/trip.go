package model

import "github.com/google/uuid"

type IdempotencyRecord struct {
	RequestHash []byte
	TripID      uuid.UUID
}
