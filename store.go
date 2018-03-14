package channelstats

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	hc "cirello.io/HumorChecker"
	"github.com/dgraph-io/badger"
	"github.com/nlopes/slack"
	"github.com/pkg/errors"
)

const hourLayout = "2006-01-02T15"

type Store struct {
	db *badger.DB
}

type DataPoint struct {
	Key   string
	Value string
}

type UserSums struct {
	User string
	Sum  int64
}

func NewStore() (*Store, error) {
	opts := badger.DefaultOptions
	opts.Dir = "./badger"
	opts.ValueDir = "./badger"
	opts.SyncWrites = true

	db, err := badger.Open(opts)
	if err != nil {
		return nil, errors.Wrap(err, "while opening badger database")
	}
	return &Store{db: db}, nil
}

func (s *Store) CountMessage(ev *slack.MessageEvent) error {
	float, err := strconv.ParseFloat(ev.Timestamp, 64)
	if err != nil {
		return errors.Wrapf(err, "timestamp conversion for '%s'", ev.Timestamp)
	}
	timestamp := time.Unix(0, int64(float*1000000)*int64(time.Microsecond/time.Nanosecond)).UTC()
	hour := timestamp.Format(hourLayout)

	// Start a badger transaction
	return s.db.Update(func(txn *badger.Txn) error {
		err := incDataPoint(txn, hour, "messages", ev.Channel, ev.User)
		if err != nil {
			errors.Wrapf(err, "while storing 'messages' datapoint")
		}

		result := hc.Analyze(ev.Text)
		fmt.Printf("Result: %+v\n", result)
		if result.Score > 0 {
			err = incDataPoint(txn, hour, "positive", ev.Channel, ev.User)
			if err != nil {
				errors.Wrapf(err, "while storing 'positive' datapoint")
			}
		}
		if result.Score < 0 {
			incDataPoint(txn, hour, "negative", ev.Channel, ev.User)
			if err != nil {
				errors.Wrapf(err, "while storing 'negative' datapoint")
			}
		}
		return nil
	})
}

func (s *Store) GetDataPoints(timeRange *TimeRange, typ, channelID string) ([]DataPoint, error) {
	var results []DataPoint

	for _, hour := range timeRange.ByHour() {
		data, err := s.GetByPrefix(hour, typ, channelID)
		if err != nil {
			return nil, errors.Wrapf(err, "during while getting data points for '%s'", hour)
		}

		if len(data) != 0 {
			results = append(results, data...)
		}
	}
	return results, nil
}

func (s Store) GetByPrefix(keys ...string) ([]DataPoint, error) {
	var results []DataPoint
	prefix := []byte(strings.Join(keys, "/"))

	err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			k := item.Key()
			v, err := item.Value()
			if err != nil {
				return err
			}
			results = append(results, DataPoint{Key: string(k), Value: string(v)})
		}
		return nil
	})
	return results, err
}

func (s *Store) GetAll() ([]DataPoint, error) {
	var results []DataPoint

	// Fetch all the things from the database
	err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			k := item.Key()
			v, err := item.Value()
			if err != nil {
				return err
			}

			results = append(results, DataPoint{Key: string(k), Value: string(v)})
		}
		return nil
	})
	return results, err
}

func (s *Store) Close() {
	s.db.Close()
}

func incDataPoint(txn *badger.Txn, keys ...string) error {
	key := []byte(strings.Join(keys, "/"))

	// Fetch this data point from the store
	item, err := txn.Get(key)
	if err != nil {
		if err != badger.ErrKeyNotFound {
			return errors.Wrapf(err, "while fetching key '%s'", key)
		}
	}

	var counter int64
	// If value exists in the store, retrieve the current counter
	if item != nil {
		value, err := item.Value()
		if err != nil {
			return errors.Wrapf(err, "while fetching counter value '%s'", key)
		}
		counter, err = strconv.ParseInt(string(value), 10, 64)
	}
	// Increment our counter
	counter += 1
	err = txn.Set(key, []byte(fmt.Sprintf("%d", counter)))
	if err != nil {
		return errors.Wrapf(err, "while setting counter for key '%s'", key)
	}
	return err
}
