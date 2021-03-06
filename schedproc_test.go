package gomes

import (
	"bytes"
	"code.google.com/p/goprotobuf/proto"
	mesos "github.com/vladimirvivien/gomes/mesosproto"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
)

func TestNewSchedID(t *testing.T) {
	re1 := regexp.MustCompile(`^[a-z]+\(\d+\)@.*$`)
	id1 := newSchedProcID(":5000")
	if !re1.MatchString(string(id1.value)) {
		t.Error("SchedID not generated properly:", id1.value)
	}

	id2 := newSchedProcID(":6000")
	re2 := regexp.MustCompile(`^[a-z]+\(\d+\)@.*$`)
	if !re2.MatchString(string(id2.value)) {
		t.Error("SchedID not generated properly.  Expected prefix scheduler(2):", id1.value)
	}
	id3 := newSchedProcID(":7000")
	re3 := regexp.MustCompile(`^[a-z]+\(\d+\)$`)
	if !re3.MatchString(id3.prefix) {
		t.Error("SchedID has invalid prefix:", id3.prefix)
	}
}

func TestNewFullSchedID(t *testing.T) {
	re1 := regexp.MustCompile(`scheduler\(\d+\)@machine1:4040`)
	id1 := newSchedProcID("machine1:4040")
	if !re1.MatchString(id1.value) {
		t.Errorf("Expecting SchedID [%s], but got [%s]", `scheduler\(\d+\)@machine1:4040`, id1.value)
	}
}

func TestSchedProcCreation(t *testing.T) {
	proc, err := newSchedulerProcess(make(chan interface{}))
	if err != nil {
		t.Fatal(err)
	}
	if proc.server == nil {
		t.Error("SchedHttpProcess missing server")
	}
}

func TestSchedProcStart(t *testing.T) {
	eventQ := make(chan interface{})
	go func() {
		msg := <-eventQ
		if val, ok := msg.(error); ok {
			t.Fatalf("TestSchedProcStart() - got error: %s", val.Error())
		}
	}()

	proc, err := newSchedulerProcess(eventQ)
	if err != nil {
		t.Fatal(err)
	}

	http.HandleFunc("/test", func(rsp http.ResponseWriter, req *http.Request) {
		rsp.WriteHeader(http.StatusAccepted)
	})

	err = proc.start()
	if err != nil {
		t.Fatalf("Error starting SchedProc %s", err)
	}

	rsp, err := http.Get("http://" + proc.listener.Addr().String() + "/test")
	if err != nil {
		t.Fatal("Error while verifying SchedProc.Server:", err)
	}
	if rsp.StatusCode != http.StatusAccepted {
		t.Log("Did not receive expected status from SchedProc /test path.")
	}
}

func TestSchedProcStop(t *testing.T) {
	eventQ := make(chan interface{})
	go func() {
		msg := <-eventQ
		if val, ok := msg.(*net.OpError); ok {

			t.Fatalf("TestSchedProcStop() - got : %s", val.Op)
		}
	}()

	proc, err := newSchedulerProcess(eventQ)
	if err != nil {
		t.Fatal(err)
	}

	err = proc.start()
	if err != nil {
		t.Fatalf("Error starting sched proc: %s", err.Error())
	}
	_, err = http.Get("http://" + proc.listener.Addr().String())
	if err != nil {
		t.Fatal("SchedProc.Server validation error:", err)
	}

	err = proc.stop()
	if err != nil {
		t.Fatal("Error stopping sched proc:", err)
	}
	_, err = http.Get("http://" + proc.listener.Addr().String())
	if err == nil {
		t.Fatal("SchedProc.Server - expected no connection, but connected OK.")
	}
}

func TestScheProcError(t *testing.T) {
	eventQ := make(chan interface{})
	go func() {
		msg := <-eventQ
		if val, ok := msg.(error); !ok {
			t.Fatalf("Expected message of type error, but got %T", msg)
		} else {
			log.Println("*** Error in Q", val, "***")
		}
	}()

	proc, err := newSchedulerProcess(eventQ)
	if err != nil {
		t.Fatal(err)
	}
	req := buildHttpRequest(t, "FrameworkRegisteredMessage", nil)
	resp := httptest.NewRecorder()
	proc.ServeHTTP(resp, req)
}

func TestFrameworkRegisteredMessage(t *testing.T) {
	// setup chanel to receive unmarshalled message
	eventQ := make(chan interface{})
	go func() {
		for msg := range eventQ {
			val, ok := msg.(*mesos.FrameworkRegisteredMessage)
			if !ok {
				t.Fatal("Failed to receive msg of type FrameworkRegisteredMessage")
			}
			if val.FrameworkId.GetValue() != "test-framework-1" {
				t.Fatal("Expected FrameworkRegisteredMessage.Framework.Id.Value not found.")
			}
			if val.MasterInfo.GetId() != "master-1" {
				t.Fatal("Expected FrameworkRegisteredMessage.Master.Id not found.")
			}
		}
	}()

	// Simulate FramworkRegisteredMessage request from master.
	proc, err := newSchedulerProcess(eventQ)
	if err != nil {
		t.Fatal(err)
	}
	proc.started = true
	proc.aborted = false
	if err != nil {
		t.Fatal("Unable to start Scheduler Process.	")
	}
	msg := &mesos.FrameworkRegisteredMessage{
		FrameworkId: NewFrameworkID("test-framework-1"),
		MasterInfo:  NewMasterInfo("master-1", 12356, 12345),
	}
	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("Unable to marshal FrameworkRegisteredMessage, %v", err)
	}

	req := buildHttpRequest(t, "FrameworkRegisteredMessage", data)
	resp := httptest.NewRecorder()

	// ServeHTTP will unmarshal msg and place on passed channel (above)
	proc.ServeHTTP(resp, req)

	if resp.Code != http.StatusAccepted {
		t.Fatalf("Expecting server status %d but got status %d", http.StatusAccepted, resp.Code)
	}
}

func TestFrameworkReRegisteredMessage(t *testing.T) {
	// setup chanel to receive unmarshalled message
	eventQ := make(chan interface{})
	go func() {
		for msg := range eventQ {
			val, ok := msg.(*mesos.FrameworkReregisteredMessage)
			if !ok {
				t.Fatal("Failed to receive msg of type FrameworkReregisteredMessage")
			}
			if val.MasterInfo.GetId() != "master-1" {
				t.Fatal("Expected FrameworkRegisteredMessage.Master.Id not found. Got", val)
			}
		}
	}()

	// Simulate FramworkReregisteredMessage request from master.
	proc, err := newSchedulerProcess(eventQ)
	if err != nil {
		t.Fatal(err)
	}
	proc.started = true
	proc.aborted = false

	msg := &mesos.FrameworkRegisteredMessage{
		FrameworkId: NewFrameworkID("test-framework-1"),
		MasterInfo:  NewMasterInfo("master-1", 123456, 12345),
	}
	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("Unable to marshal FrameworkReregisteredMessage, %v", err)
	}

	req := buildHttpRequest(t, "FrameworkReregisteredMessage", data)
	resp := httptest.NewRecorder()

	// ServeHTTP will unmarshal msg and place on passed channel (above)
	proc.ServeHTTP(resp, req)

	if resp.Code != http.StatusAccepted {
		t.Fatalf("Expecting server status %d but got status %d", http.StatusAccepted, resp.Code)
	}
}

func TestResourceOffersMessage(t *testing.T) {
	eventQ := make(chan interface{})
	go func() {
		for msg := range eventQ {
			val, ok := msg.(*mesos.ResourceOffersMessage)
			if !ok {
				t.Fatal("Failed to receive msg of type ResourceOffersMessage")
			}
			if len(val.Offers) != 1 {
				t.Fatal("SchedProc not receiving ResourceOffersMessage properly. ")
			}
		}
	}()

	proc, err := newSchedulerProcess(eventQ)
	if err != nil {
		t.Fatal(err)
	}
	proc.started = true
	proc.aborted = false

	msg := &mesos.ResourceOffersMessage{
		Offers: []*mesos.Offer{
			NewOffer(NewOfferID("offer-1"),
				NewFrameworkID("test-framework-1"),
				NewSlaveID("slave-1"),
				"localhost"),
		},
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("Unable to marshal ResourceOffersMessage, %v", err)
	}

	req := buildHttpRequest(t, "ResourceOffersMessage", data)
	resp := httptest.NewRecorder()

	// ServeHTTP will unmarshal msg and place on passed channel (above)
	proc.ServeHTTP(resp, req)

	if resp.Code != http.StatusAccepted {
		t.Fatalf("Expecting server status %d but got status %d", http.StatusAccepted, resp.Code)
	}
}

func TestRescindOfferMessage(t *testing.T) {
	eventQ := make(chan interface{})
	go func() {
		for msg := range eventQ {
			val, ok := msg.(*mesos.RescindResourceOfferMessage)
			if !ok {
				t.Fatal("Failed to receive msg of type RescindResourceOfferMessage")
			}
			if val.OfferId.GetValue() != "offer-2" {
				t.Fatal("Expected value not found in RescindResourceOfferMessage. See HTTP handler.")
			}
		}
	}()

	proc, err := newSchedulerProcess(eventQ)
	if err != nil {
		t.Fatal(err)
	}
	proc.started = true
	proc.aborted = false

	msg := &mesos.RescindResourceOfferMessage{
		OfferId: &mesos.OfferID{Value: proto.String("offer-2")},
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("Unable to marshal RescindResourceOfferMessage, %v", err)
	}

	req := buildHttpRequest(t, "RescindResourceOfferMessage", data)
	resp := httptest.NewRecorder()

	proc.ServeHTTP(resp, req)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("Expecting server status %d but got status %d", http.StatusAccepted, resp.Code)
	}

}

func TestStatusUpdateMessage(t *testing.T) {
	eventQ := make(chan interface{})
	go func() {
		for msg := range eventQ {
			val, ok := msg.(*mesos.StatusUpdateMessage)
			if !ok {
				t.Fatal("Failed to receive msg of type StatusUpdateMessage")
			}

			if val.Update.FrameworkId.GetValue() != "test-framework-1" {
				t.Fatal("Expected StatusUpdateMessage.FramewId not received.")
			}

			if val.Update.Status.GetState() != mesos.TaskState(mesos.TaskState_TASK_RUNNING) {
				t.Fatal("Expected StatusUpdateMessage.Update.Status.State not received.")
			}

			if string(val.Update.Status.GetData()) != "World!" {
				t.Fatal("Expected StatusUpdateMessage.Update.Message not received.")
			}

		}
	}()

	proc, err := newSchedulerProcess(eventQ)
	if err != nil {
		t.Fatal(err)
	}
	proc.started = true
	proc.aborted = false

	status := NewTaskStatus(NewTaskID("task-1"), mesos.TaskState_TASK_RUNNING)
	status.Data = []byte("World!")
	msg := &mesos.StatusUpdateMessage{
		Update: NewStatusUpdate(
			NewFrameworkID("test-framework-1"),
			status,
			123456789.1,
			[]byte("abcd-efg1-2345-6789-abcd-efg1")),
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("Unable to marshal StatusUpdateMessage, %v", err)
	}

	req := buildHttpRequest(t, "StatusUpdateMessage", data)
	resp := httptest.NewRecorder()

	proc.ServeHTTP(resp, req)

	if resp.Code != http.StatusAccepted {
		t.Fatalf("Expecting server status %d but got status %d", http.StatusAccepted, resp.Code)
	}
}

func TestFrameworkMessage(t *testing.T) {
	eventQ := make(chan interface{})
	go func() {
		for msg := range eventQ {
			val, ok := msg.(*mesos.ExecutorToFrameworkMessage)
			if !ok {
				t.Fatal("Failed to receive msg of type ExecutorToFrameworkMessage")
			}
			if val.SlaveId.GetValue() != "test-slave-1" {
				t.Fatal("ExecutorToFrameworkMessage.SlaveId not received.")
			}
			if string(val.GetData()) != "Hello-Test" {
				t.Fatal("ExecutorToFrameworkMessage.Data not received.")
			}
		}
	}()

	proc, err := newSchedulerProcess(eventQ)
	if err != nil {
		t.Fatal(err)
	}
	proc.started = true
	proc.aborted = false

	msg := &mesos.ExecutorToFrameworkMessage{
		SlaveId:     &mesos.SlaveID{Value: proto.String("test-slave-1")},
		FrameworkId: &mesos.FrameworkID{Value: proto.String("test-framework-1")},
		ExecutorId:  &mesos.ExecutorID{Value: proto.String("test-executor-")},
		Data:        []byte("Hello-Test"),
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("Unable to marshal ExecutorToFrameworkMessage, %v", err)
	}

	req := buildHttpRequest(t, "ExecutorToFrameworkMessage", data)
	resp := httptest.NewRecorder()

	proc.ServeHTTP(resp, req)

	if resp.Code != http.StatusAccepted {
		t.Fatalf("Expecting server status %d but got status %d", http.StatusAccepted, resp.Code)
	}
}

func TestLostSlaveMessage(t *testing.T) {
	eventQ := make(chan interface{})
	go func() {
		for msg := range eventQ {
			val, ok := msg.(*mesos.LostSlaveMessage)
			if !ok {
				t.Fatal("Failed to receive msg of type ExecutorToFrameworkMessage")
			}
			if val.SlaveId.GetValue() != "test-slave-1" {
				t.Fatal("LostSlaveMessage.SlaveId not received.")
			}
		}
	}()

	proc, err := newSchedulerProcess(eventQ)
	if err != nil {
		t.Fatal(err)
	}
	proc.started = true
	proc.aborted = false

	msg := &mesos.LostSlaveMessage{SlaveId: &mesos.SlaveID{Value: proto.String("test-slave-1")}}

	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("Unable to marshal LostSlaveMessage, %v", err)
	}

	req := buildHttpRequest(t, LOST_SLAVE_EVENT, data)
	resp := httptest.NewRecorder()

	proc.ServeHTTP(resp, req)

	if resp.Code != http.StatusAccepted {
		t.Fatalf("Expecting server status %d but got status %d", http.StatusAccepted, resp.Code)
	}

}

func buildHttpRequest(t *testing.T, msgName string, data []byte) *http.Request {
	u, _ := address("127.0.0.1:5151").AsFullHttpURL(
		"/scheduler(1)/" + MESOS_INTERNAL_PREFIX + msgName)
	req, err := http.NewRequest(HTTP_POST_METHOD, u.String(), bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Add("Content-Type", HTTP_CONTENT_TYPE)
	req.Header.Add("Connection", "Keep-Alive")
	req.Header.Add("Libprocess-From", "master(1)")
	return req
}
