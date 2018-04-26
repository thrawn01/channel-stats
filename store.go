package channelstats

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	hc "cirello.io/HumorChecker"
	"github.com/dgraph-io/badger"
	"github.com/nlopes/slack"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var linkRegex = regexp.MustCompile(`(http://|https://)`)
var emojiRegex = regexp.MustCompile(`:([a-z0-9_\+\-]+):`)

type Store struct {
	idMgr *IDManager
	log   *logrus.Entry
	db    *badger.DB
}

func NewStore(idMgr *IDManager) (*Store, error) {
	opts := badger.DefaultOptions
	opts.Dir = "./badger-db"
	opts.ValueDir = "./badger-db"
	opts.SyncWrites = true

	db, err := badger.Open(opts)
	if err != nil {
		return nil, errors.Wrap(err, "while opening badger database")
	}
	return &Store{
		log:   log.WithField("prefix", "store"),
		idMgr: idMgr,
		db:    db,
	}, nil
}

type DataPoint struct {
	Hour        string
	UserID      string
	UserName    string
	ChannelID   string
	ChannelName string
	DataType    string
	Value       int64
}

func DataPointFrom(item *badger.Item) (DataPoint, error) {
	parts := strings.Split(string(item.Key()), "/")

	value, err := item.Value()
	if err != nil {
		return DataPoint{}, errors.Wrap(err, "while converting back to a data point; item.Value() returned")
	}

	// Decode the int
	valueInt, err := strconv.ParseInt(string(value), 10, 64)
	//valueInt, _ := binary.Varint(value)

	return DataPoint{
		Hour:      parts[0],
		DataType:  parts[1],
		ChannelID: parts[2],
		UserID:    parts[3],
		Value:     valueInt,
	}, nil
}

func (s *DataPoint) Key() []byte {
	return []byte(fmt.Sprintf("%s/%s/%s/%s", s.Hour, s.DataType, s.ChannelID, s.UserID))
}

func (s *DataPoint) PrefixKey() []byte {
	return []byte(fmt.Sprintf("%s/%s/%s", s.Hour, s.DataType, s.ChannelID))
}

func (s *DataPoint) ResolveID(idMgr *IDManager) (err error) {
	s.ChannelName, err = idMgr.GetChannelName(s.ChannelID)
	s.UserName, err = idMgr.GetUserName(s.UserID)
	return err
}

func (s *DataPoint) EncodeValue() []byte {
	//buf := make([]byte, binary.MaxVarintLen64)
	//n := binary.PutVarint(buf, s.Value)
	//return buf[:n]
	return []byte(fmt.Sprintf("%d", s.Value))
}

func (s *Store) GetDataPoints(timeRange *TimeRange, dataType, channelID string) ([]DataPoint, error) {
	s.log.Debugf("GetDataPoints(%+v, %s, %s)", *timeRange, dataType, channelID)
	var results []DataPoint

	for _, hour := range timeRange.ByHour() {
		dp := DataPoint{Hour: hour, DataType: dataType, ChannelID: channelID}
		data, err := s.GetByPrefix(dp.PrefixKey())
		if err != nil {
			return nil, errors.Wrapf(err, "during while getting data points for prefix '%s'", dp.PrefixKey())
		}

		if len(data) != 0 {
			results = append(results, data...)
		}
	}
	return results, nil
}

type UserSum struct {
	User string
	Sum  int64
}

func (s *Store) SumByUser(timeRange *TimeRange, dataType, channelID string) ([]UserSum, error) {
	var results []UserSum

	dataPoints, err := s.GetDataPoints(timeRange, dataType, channelID)
	if err != nil {
		return nil, err
	}

	byUser := make(map[string]int64)
	for _, dp := range dataPoints {
		if value, exists := byUser[dp.UserName]; exists {
			byUser[dp.UserName] = value + dp.Value
		} else {
			byUser[dp.UserName] = dp.Value
		}
	}

	for key, value := range byUser {
		results = append(results, UserSum{User: key, Sum: value})
	}

	// Sort the results
	sort.Slice(results, func(i, j int) bool {
		return results[i].Sum < results[j].Sum
	})

	return results, nil
}

func (s Store) GetByPrefix(keyPrefix []byte) ([]DataPoint, error) {
	var results []DataPoint

	err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Seek(keyPrefix); it.ValidForPrefix(keyPrefix); it.Next() {
			dp, err := DataPointFrom(it.Item())
			if err != nil {
				return err
			}
			if err = dp.ResolveID(s.idMgr); err != nil {
				s.log.Debugf("while resolving data point ids for '%+v': %s", dp, err)
			}
			results = append(results, dp)
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
			dp, err := DataPointFrom(it.Item())
			if err != nil {
				return err
			}
			if err = dp.ResolveID(s.idMgr); err != nil {
				s.log.Debugf("while resolving data point ids for '%+v': %s", dp, err)
			}
			results = append(results, dp)
		}
		return nil
	})
	return results, err
}

func (s *Store) Close() {
	s.db.Close()
}

func (s *Store) HourFromTimeStamp(text string) (string, error) {
	float, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return "", errors.Wrapf(err, "timestamp conversion for '%s'", text)
	}
	timestamp := time.Unix(0, int64(float*1000000)*int64(time.Microsecond/time.Nanosecond)).UTC()
	return timestamp.Format(RFC3339Short), nil
}

func (s *Store) HandleReactionAdded(ev *slack.ReactionAddedEvent) error {
	hour, err := s.HourFromTimeStamp(ev.EventTimestamp)
	if err != nil {
		return errors.Wrap(err, "while handling reaction added")
	}
	dp := DataPoint{
		Hour:      hour,
		ChannelID: ev.Item.Channel,
		UserID:    ev.User,
		Value:     int64(1),
	}

	return s.db.Update(func(txn *badger.Txn) error {
		dp.DataType = "emoji"
		err := saveDataPoint(txn, dp)
		if err != nil {
			errors.Wrapf(err, "while storing 'messages' data point")
		}
		return nil
	})
}

func (s *Store) HandleMessage(ev *slack.MessageEvent) error {
	hour, err := s.HourFromTimeStamp(ev.Timestamp)
	if err != nil {
		return errors.Wrap(err, "while handling message")
	}

	// Silently ignore empty messages
	if len(ev.Text) == 0 {
		return nil
	}

	dp := DataPoint{
		Hour:      hour,
		ChannelID: ev.Channel,
		UserID:    ev.User,
		Value:     int64(1),
	}

	// Start a badger transaction
	return s.db.Update(func(txn *badger.Txn) error {

		// Count Messages
		dp.DataType = "messages"
		err := saveDataPoint(txn, dp)
		if err != nil {
			errors.Wrapf(err, "while storing 'messages' data point")
		}

		// Sentiment Analysis
		result := hc.Analyze(ev.Text)
		if result.Score > 0 {
			dp.DataType = "positive"
			err = saveDataPoint(txn, dp)
			if err != nil {
				errors.Wrapf(err, "while storing 'positive' data point")
			}
		}
		if result.Score < 0 {
			dp.DataType = "negative"
			saveDataPoint(txn, dp)
			if err != nil {
				errors.Wrapf(err, "while storing 'negative' data point")
			}
		}

		// Link counter
		if HasLink(ev.Text) {
			dp.DataType = "link"
			saveDataPoint(txn, dp)
			if err != nil {
				errors.Wrapf(err, "while storing 'link' data point")
			}
		}

		// Emoji counter
		if HasEmoji(ev.Text) {
			dp.DataType = "emoji"
			saveDataPoint(txn, dp)
			if err != nil {
				errors.Wrapf(err, "while storing 'emoji' data point")
			}
		}

		// Count words
		count := CountWords(ev.Text)
		dp.DataType = "word-count"
		dp.Value = count
		saveDataPoint(txn, dp)
		if err != nil {
			errors.Wrapf(err, "while storing 'word-count' data point")
		}

		return nil
	})
}

const (
	OUT = 0
	IN  = 1
)

func CountWords(text string) int64 {
	state := OUT
	var wc int64

	// Scan all characters one by one
	for i := 0; i < len(text); i++ {
		// If next character is a separator, set the state as OUT
		if text[i] == ' ' || text[i] == '\n' || text[i] == '\t' {
			state = OUT
		} else if state == OUT {
			// If next character is not a word separator and state is OUT,
			// then set the state as IN and increment word count
			state = IN
			wc += 1
		}
	}
	return wc
}

func HasLink(text string) bool {
	return linkRegex.MatchString(text)
}

func HasEmoji(text string) bool {
	return emojiRegex.MatchString(text)
}

func saveDataPoint(txn *badger.Txn, dp DataPoint) error {
	key := dp.Key()

	// Fetch data point from the store if it exists
	item, err := txn.Get(key)
	if err != nil {
		if err != badger.ErrKeyNotFound {
			return errors.Wrapf(err, "while fetching key '%s'", key)
		}
	}

	// If data point exists in the store, retrieve the current data point
	if item != nil {
		dpCurrent, err := DataPointFrom(item)
		if err != nil {
			return errors.Wrapf(err, "while fetching counter value '%s'", key)
		}
		// Add to our current value
		dp.Value += dpCurrent.Value
	}

	err = txn.Set(key, dp.EncodeValue())
	if err != nil {
		return errors.Wrapf(err, "while setting counter for key '%s'", key)
	}
	return err
}
