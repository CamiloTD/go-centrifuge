// +build unit

package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/centrifuge/go-centrifuge/bootstrap"
	"github.com/centrifuge/go-centrifuge/bootstrap/bootstrappers/testlogging"
	"github.com/centrifuge/go-centrifuge/config"
	"github.com/centrifuge/go-centrifuge/contextutil"
	"github.com/centrifuge/go-centrifuge/utils"
	"github.com/stretchr/testify/assert"
)

var cfg config.Configuration

func TestMain(m *testing.M) {
	ibootstappers := []bootstrap.TestBootstrapper{
		&testlogging.TestLoggingBootstrapper{},
		&config.Bootstrapper{},
	}
	ctx := make(map[string]interface{})
	bootstrap.RunTestBootstrappers(ibootstappers, ctx)
	cfg = ctx[bootstrap.BootstrappedConfig].(config.Configuration)
	result := m.Run()
	bootstrap.RunTestTeardown(ibootstappers)
	os.Exit(result)
}

func sendAndVerify(t *testing.T, message Message) {
	var wg sync.WaitGroup
	wg.Add(1)
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", func(writer http.ResponseWriter, request *http.Request) {
		var resp Message
		defer request.Body.Close()
		defer wg.Done()
		data, err := ioutil.ReadAll(request.Body)
		assert.NoError(t, err)

		err = json.Unmarshal(data, &resp)
		assert.NoError(t, err)
		writer.Write([]byte("success"))
		assert.Equal(t, message.EventType, resp.EventType)
		if message.EventType == EventTypeJob {
			assert.Equal(t, *message.Job, *resp.Job)
			assert.Nil(t, resp.Document)
		} else {
			assert.Equal(t, *message.Document, *resp.Document)
			assert.Nil(t, resp.Job)
		}
	})

	addr, _, err := utils.GetFreeAddrPort()
	assert.NoError(t, err)
	server := &http.Server{Addr: addr, Handler: mux}
	go server.ListenAndServe()
	defer server.Close()

	wb := NewWebhookSender()
	url := fmt.Sprintf("http://%s/webhook", addr)
	cfg.Set("notifications.endpoint", url)
	acc := new(config.MockAccount)
	acc.On("GetReceiveEventNotificationEndpoint").Return(url).Once()
	ctx, err := contextutil.New(context.Background(), acc)
	assert.NoError(t, err)

	err = wb.Send(ctx, message)
	assert.NoError(t, err)
	wg.Wait()
}

func TestWebhookSender_JobUpdate(t *testing.T) {
	message := Message{
		EventType:  EventTypeJob,
		RecordedAt: time.Now().UTC(),
		Job: &JobMessage{
			ID:         utils.RandomSlice(32),
			Owner:      utils.RandomSlice(20),
			Desc:       "Sample Job",
			ValidUntil: time.Now().Add(time.Hour).UTC(),
			FinishedAt: time.Now().UTC(),
		},
	}
	sendAndVerify(t, message)
}

func TestWebhookSender_DocumentUpdate(t *testing.T) {
	message := Message{
		EventType:  EventTypeDocument,
		RecordedAt: time.Now().UTC(),
		Document: &DocumentMessage{
			ID:        utils.RandomSlice(32),
			VersionID: utils.RandomSlice(32),
			From:      utils.RandomSlice(20),
			To:        utils.RandomSlice(20),
		},
	}
	sendAndVerify(t, message)
}
