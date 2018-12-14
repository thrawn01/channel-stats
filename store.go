package channelstats

import (
	"fmt"
	"log"
	"regexp"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"time"

	hc "cirello.io/HumorChecker"
	"github.com/dgraph-io/badger"
	"github.com/mailgun/holster"
	"github.com/nlopes/slack"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	OUT = 0
	IN  = 1
)

var linkRegex = regexp.MustCompile(`(http://|https://)`)
var emojiRegex = regexp.MustCompile(`:([a-z0-9_\+\-]+):`)

type Storer interface {
	PercentageByUser(timeRange *TimeRange, dataType, channelID string) ([]PercentageResp, error)
	GetDataPoints(*TimeRange, string, string) ([]DataPoint, error)
	SumByUser(*TimeRange, string, string) ([]SumResp, error)
	HandleReactionAdded(*slack.ReactionAddedEvent) error
	HandleMessage(*slack.MessageEvent) error
	GetAll() ([]DataPoint, error)
	Close() error
}

type Store struct {
	idMgr IDManager
	log   *logrus.Entry
	db    *badger.DB
}

func NewStore(conf Config, idMgr IDManager) (Storer, error) {
	opts := badger.DefaultOptions
	opts.Dir = conf.Store.DataDir
	opts.ValueDir = conf.Store.DataDir
	opts.SyncWrites = true

	// Badger logs to std logging, send logs to logrus instead
	logger := GetLogger().WithField("prefix", "store")
	log.SetOutput(logger.Writer())

	db, err := badger.Open(opts)
	if err != nil {
		return nil, errors.Wrap(err, "while opening badger database")
	}
	return &Store{
		log:   logger,
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

func (s DataPoint) PrefixKey() []byte {
	return []byte(fmt.Sprintf("%s/%s/%s", s.Hour, s.DataType, s.ChannelID))
}

func (s *DataPoint) ResolveID(idMgr IDManager) (err error) {
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
	resultChan := make(chan DataPoint, 5)

	go func() {
		fan := holster.NewFanOut(5)
		for _, hour := range timeRange.ByHour() {
			fan.Run(func(data interface{}) error {
				key := DataPoint{Hour: data.(string), DataType: dataType, ChannelID: channelID}.PrefixKey()
				dps, err := s.GetByPrefix(key)
				if err != nil {
					return errors.Wrapf(err, "during while getting data points for prefix '%s'", key)
				}
				if len(dps) != 0 {
					for _, dp := range dps {
						resultChan <- dp
					}
				}
				return nil
			}, hour)
		}
		fan.Wait()
		close(resultChan)
	}()

	var results []DataPoint
	for dp := range resultChan {
		results = append(results, dp)
	}
	return results, nil
}

type SumResp struct {
	User string `json:"user"`
	Sum  int64  `json:"sum"`
}

func (s *Store) SumByUser(timeRange *TimeRange, dataType, channelID string) ([]SumResp, error) {
	var results []SumResp

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
		results = append(results, SumResp{User: key, Sum: value})
	}

	// Sort the results
	sort.Slice(results, func(i, j int) bool {
		return results[i].Sum < results[j].Sum
	})

	return results, nil
}

type PercentageResp struct {
	// Name of the user
	User string `json:"user"`
	// Total number of messages used in the percent calculation
	Total int64 `json:"total"`
	// Total number of the 'counter' used in the percent calculation
	Count int64 `json:"count"`
	// The calculated percentage
	Percent int64 `json:"percentage"`
}

func (s *Store) PercentageByUser(timeRange *TimeRange, dataType, channelID string) ([]PercentageResp, error) {
	// Get the total number of messages for the channel during this time
	messages, err := s.SumByUser(timeRange, "messages", channelID)
	if err != nil {
		return nil, err
	}

	// Get the data type counts during this time
	counters, err := s.SumByUser(timeRange, dataType, channelID)
	if err != nil {
		return nil, err
	}

	counterMap := make(map[string]int64)
	for _, counter := range counters {
		counterMap[counter.User] = counter.Sum
	}

	var results []PercentageResp
	for _, message := range messages {
		count, ok := counterMap[message.User]
		if ok {
			results = append(results, PercentageResp{
				User:    message.User,
				Total:   message.Sum,
				Count:   count,
				Percent: int64((float64(count) / float64(message.Sum)) * 100),
			})
		}
	}

	// Sort the results by number of messages. We do this so the more accurate
	// percentages appear at the top of the list
	sort.Slice(results, func(i, j int) bool {
		return results[i].Total < results[j].Total
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

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) hourFromTimeStamp(text string) (string, error) {
	float, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return "", errors.Wrapf(err, "timestamp conversion for '%s'", text)
	}
	timestamp := time.Unix(0, int64(float*1000000)*int64(time.Microsecond/time.Nanosecond)).UTC()
	return timestamp.Format(RFC3339Short), nil
}

func (s *Store) HandleReactionAdded(ev *slack.ReactionAddedEvent) error {
	hour, err := s.hourFromTimeStamp(ev.EventTimestamp)
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
			return errors.Wrapf(err, "while storing 'messages' data point")
		}
		return nil
	})
}

func (s *Store) HandleMessage(ev *slack.MessageEvent) error {
	hour, err := s.hourFromTimeStamp(ev.Timestamp)
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
			return errors.Wrapf(err, "while storing 'messages' data point")
		}

		result := SentimentAnalysis(ev.Text)
		if result.Score > 0 {
			dp.DataType = "positive"
			err = saveDataPoint(txn, dp)
			if err != nil {
				return errors.Wrapf(err, "while storing 'positive' data point")
			}
		}
		if result.Score < 0 {
			dp.DataType = "negative"
			if err = saveDataPoint(txn, dp); err != nil {
				return errors.Wrapf(err, "while storing 'negative' data point")
			}
		}

		// Link counter
		if HasLink(ev.Text) {
			dp.DataType = "link"
			if err = saveDataPoint(txn, dp); err != nil {
				return errors.Wrapf(err, "while storing 'link' data point")
			}
		}

		// Emoji counter
		if HasEmoji(ev.Text) {
			dp.DataType = "emoji"
			if err = saveDataPoint(txn, dp); err != nil {
				return errors.Wrapf(err, "while storing 'emoji' data point")
			}
		}

		// Count words
		count := CountWords(ev.Text)
		dp.DataType = "word-count"
		dp.Value = count
		if err = saveDataPoint(txn, dp); err != nil {
			return errors.Wrapf(err, "while storing 'word-count' data point")
		}
		return nil
	})
}

func SentimentAnalysis(message string) (score hc.FullScore) {
	defer func() {
		// Sentiment Analysis Panics often....
		if r := recover(); r != nil {
			fmt.Printf("-- Caught PANIC in SentimentAnalysis()")
			debug.PrintStack()
			score = hc.FullScore{}
		}
	}()
	score = hc.Analyze(message)
	return
}

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

// Suitable for testing
type NullStore struct{}

func (n *NullStore) GetDataPoints(*TimeRange, string, string) ([]DataPoint, error) {
	return []DataPoint{}, nil
}
func (n *NullStore) SumByUser(*TimeRange, string, string) ([]SumResp, error) { return []SumResp{}, nil }
func (n *NullStore) GetAll() ([]DataPoint, error)                            { return []DataPoint{}, nil }
func (n *NullStore) Close() error                                            { return nil }
