package models

import (
	"context"
	"crypto/x509"
	"encoding/hex"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/usmbest/ocean.one/example/durable"
	"github.com/usmbest/ocean.one/example/session"
	"google.golang.org/api/iterator"
)

var usersColumnsFull = []string{"user_id", "email", "phone", "mixin_id", "identity_id", "full_name", "encrypted_password", "active_at", "created_at"}

func (u *User) valuesFull() []interface{} {
	return []interface{}{u.UserId, u.Email, u.Phone, u.MixinId, u.IdentityId, u.FullName, u.EncryptedPassword, u.ActiveAt, u.CreatedAt}
}

type User struct {
	UserId            string
	Email             spanner.NullString
	Phone             spanner.NullString
	MixinId           spanner.NullString
	IdentityId        spanner.NullString
	FullName          string
	EncryptedPassword string
	ActiveAt          time.Time
	CreatedAt         time.Time

	SessionId string
	Key       *Key
}

func CreateOrResetUser(ctx context.Context, verificationId, password, sessionSecret string) (*User, error) {
	pkix, err := hex.DecodeString(sessionSecret)
	if err != nil {
		return nil, session.BadDataError(ctx)
	}
	_, err = x509.ParsePKIXPublicKey(pkix)
	if err != nil {
		return nil, session.BadDataError(ctx)
	}

	password, err = ValidateAndEncryptPassword(ctx, password)
	if err != nil {
		return nil, err
	}

	createdAt := time.Now()
	user := &User{
		EncryptedPassword: password,
		ActiveAt:          createdAt,
		CreatedAt:         createdAt,
	}

	_, err = session.Database(ctx).ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		vf, err := readVerification(ctx, txn, verificationId)
		if err != nil {
			return err
		}
		if vf == nil || !vf.VerifiedAt.Valid {
			return session.VerificationCodeInvalidError(ctx)
		}
		if vf.Category != VerificationCategoryPhone && vf.Category != VerificationCategoryEmail {
			return session.BadDataError(ctx)
		}

		user.FullName = vf.Receiver
		var userId string
		if vf.Category == VerificationCategoryPhone {
			userId, err = readUserIdByIndexKey(ctx, txn, "users_by_phone", vf.Receiver)
			if err != nil {
				return err
			}
			user.Phone = spanner.NullString{vf.Receiver, true}
		}
		if vf.Category == VerificationCategoryEmail {
			userId, err = readUserIdByIndexKey(ctx, txn, "users_by_email", vf.Receiver)
			if err != nil {
				return err
			}
			user.Email = spanner.NullString{vf.Receiver, true}
		}

		if userId != "" {
			user, err = readUser(ctx, txn, userId)
			if err != nil {
				return err
			}
			user.EncryptedPassword = password
			user.ActiveAt = createdAt
			cleanupSessions(ctx, txn, user.UserId)
		}
		var key *Key
		if userId == "" {
			key, err = consumePoolKey(ctx, txn)
			if err != nil {
				return err
			}
			if key == nil {
				return session.InsufficientKeyPoolError(ctx)
			}
			user.UserId = key.UserId
		}
		mutations := []*spanner.Mutation{
			spanner.Delete("verifications", spanner.Key{vf.VerificationId}),
			spanner.InsertOrUpdate("users", usersColumnsFull, user.valuesFull()),
		}
		if key != nil {
			mutations = append(mutations, spanner.Insert("keys", keysColumnsFull, key.valuesFull()))
		}
		err = txn.BufferWrite(mutations)
		if err != nil {
			return err
		}

		session, err := addSession(ctx, txn, user.UserId, sessionSecret)
		if err != nil {
			return err
		}
		user.SessionId = session.SessionId
		return nil
	}, "users", "INSERT", "CreateOrResetUser")

	if err != nil {
		if se, ok := session.ParseError(spanner.ErrDesc(err)); ok {
			return nil, se
		}
		return nil, session.TransactionError(ctx, err)
	}
	return user, nil
}

func (current *User) UpdateName(ctx context.Context, name string) (*User, error) {
	if len(name) > 128 {
		return nil, session.BadDataError(ctx)
	}
	err := session.Database(ctx).Apply(ctx, []*spanner.Mutation{
		spanner.Update("users", []string{"user_id", "full_name"}, []interface{}{current.UserId, name}),
	}, "users", "UPDATE", "UpdateName")
	if err != nil {
		return nil, session.TransactionError(ctx, err)
	}
	current.FullName = name
	return current, nil
}

func readUserIdByIndexKey(ctx context.Context, txn durable.Transaction, index, key string) (string, error) {
	it := txn.ReadUsingIndex(ctx, "users", index, spanner.Key{key}, []string{"user_id"})
	defer it.Stop()

	row, err := it.Next()
	if err == iterator.Done {
		return "", nil
	} else if err != nil {
		return "", err
	}

	var id string
	err = row.Columns(&id)
	return id, err
}

func readUserByPhone(ctx context.Context, txn durable.Transaction, phone string) (*User, error) {
	id, err := readUserIdByIndexKey(ctx, txn, "users_by_phone", phone)
	if err != nil || id == "" {
		return nil, err
	}

	it := txn.Read(ctx, "users", spanner.Key{id}, usersColumnsFull)
	defer it.Stop()

	row, err := it.Next()
	if err == iterator.Done {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	return userFromRow(row)
}

func readUserByEmail(ctx context.Context, txn durable.Transaction, email string) (*User, error) {
	id, err := readUserIdByIndexKey(ctx, txn, "users_by_email", email)
	if err != nil || id == "" {
		return nil, err
	}

	it := txn.Read(ctx, "users", spanner.Key{id}, usersColumnsFull)
	defer it.Stop()

	row, err := it.Next()
	if err == iterator.Done {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	return userFromRow(row)
}

func readUser(ctx context.Context, txn durable.Transaction, userId string) (*User, error) {
	it := txn.Read(ctx, "users", spanner.Key{userId}, usersColumnsFull)
	defer it.Stop()

	row, err := it.Next()
	if err == iterator.Done {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	return userFromRow(row)
}

func userFromRow(row *spanner.Row) (*User, error) {
	var u User
	err := row.Columns(&u.UserId, &u.Email, &u.Phone, &u.MixinId, &u.IdentityId, &u.FullName, &u.EncryptedPassword, &u.ActiveAt, &u.CreatedAt)
	return &u, err
}
