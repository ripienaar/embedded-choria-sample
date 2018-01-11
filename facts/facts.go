package facts

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"sync"
	"time"

	"github.com/choria-io/go-choria/choria"
	"github.com/sirupsen/logrus"
)

var f *os.File
var conf *choria.Config
var err error

type Factdata struct {
	Identity string `json:"identity"`
	Time     int64  `json:"time"`
}

func Expose(ctx context.Context, wg *sync.WaitGroup, cfg *choria.Config) {
	defer wg.Done()

	conf = cfg

	f, err = ioutil.TempFile("", "acmefacts")
	if err != nil {
		logrus.Fatalf("Could not create fact temp file: %s", err)
	}
	defer os.Remove(f.Name())

	// we will do atomic write to this file via renames, so not directly to it
	f.Close()

	// discovery will find this file now and report on its contents as json format
	cfg.Choria.FactSourceFile = f.Name()

	writer := func() {
		err := write()
		if err != nil {
			logrus.Warnf("Could not write fact data: %s", err)
		}
	}

	logrus.Infof("Writing fact data to %s", f.Name())

	writer()

	for {
		select {
		case <-time.Tick(time.Duration(60) * time.Second):
			writer()
		case <-ctx.Done():
			return
		}
	}
}

// Data returns current set of Factdata
func Data() Factdata {
	return Factdata{conf.Identity, time.Now().Unix()}
}

func write() error {
	tf, err := ioutil.TempFile("", "")
	if err != nil {
		return err
	}
	defer os.Remove(tf.Name())

	tf.Close()

	j, err := json.Marshal(Data())
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(tf.Name(), j, 0644)
	if err != nil {
		return err
	}

	err = os.Rename(tf.Name(), f.Name())
	if err != nil {
		return err
	}

	return nil
}
