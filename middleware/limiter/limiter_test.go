package limiter

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2/utils"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"
)

// go test -run Test_Limiter_Concurrency -race -v
func Test_Limiter_Concurrency(t *testing.T) {

	// Test concurrency using a default store

	app := fiber.New()

	app.Use(New(Config{
		Max:      50,
		Duration: 2 * time.Second,
	}))

	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendString("Hello tester!")
	})

	var wg sync.WaitGroup
	singleRequest := func(wg *sync.WaitGroup) {
		defer wg.Done()
		resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/", nil))
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Unexpected status code %v", resp.StatusCode)
		}
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil || "Hello tester!" != string(body) {
			t.Fatalf("Unexpected body %v", string(body))
		}
	}

	for i := 0; i <= 49; i++ {
		wg.Add(1)
		go singleRequest(&wg)
	}

	wg.Wait()

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/", nil))
	utils.AssertEqual(t, nil, err)
	utils.AssertEqual(t, 429, resp.StatusCode)

	time.Sleep(3 * time.Second)

	resp, err = app.Test(httptest.NewRequest(http.MethodGet, "/", nil))
	utils.AssertEqual(t, nil, err)
	utils.AssertEqual(t, 200, resp.StatusCode)

	// Test concurrency using a custom store

	app = fiber.New()

	app.Use(New(Config{
		Max:      50,
		Duration: 2 * time.Second,
		Store:    testStore{stmap: map[string][]byte{}, mutex: new(sync.Mutex)},
	}))

	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendString("Hello tester!")
	})

	for i := 0; i <= 49; i++ {
		wg.Add(1)
		go singleRequest(&wg)
	}

	wg.Wait()

	resp, err = app.Test(httptest.NewRequest(http.MethodGet, "/", nil))
	utils.AssertEqual(t, nil, err)
	utils.AssertEqual(t, 429, resp.StatusCode)

	time.Sleep(3 * time.Second)

	resp, err = app.Test(httptest.NewRequest(http.MethodGet, "/", nil))
	utils.AssertEqual(t, nil, err)
	utils.AssertEqual(t, 200, resp.StatusCode)

}

// go test -v -run=^$ -bench=Benchmark_Limiter -benchmem -count=4
func Benchmark_Limiter(b *testing.B) {
	app := fiber.New()

	app.Use(New(Config{
		Max:      100,
		Duration: 60 * time.Second,
	}))

	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendString("Hello, World!")
	})

	h := app.Handler()

	fctx := &fasthttp.RequestCtx{}
	fctx.Request.Header.SetMethod("GET")
	fctx.Request.SetRequestURI("/")

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		h(fctx)
	}
}

// go test -v -run=^$ -bench=Benchmark_Limiter_Custom_Store -benchmem -count=4
func Benchmark_Limiter_Custom_Store(b *testing.B) {
	app := fiber.New()

	app.Use(New(Config{
		Max:      100,
		Duration: 60 * time.Second,
		Store:    testStore{stmap: map[string][]byte{}, mutex: new(sync.Mutex)},
	}))

	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendString("Hello, World!")
	})

	h := app.Handler()

	fctx := &fasthttp.RequestCtx{}
	fctx.Request.Header.SetMethod("GET")
	fctx.Request.SetRequestURI("/")

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		h(fctx)
	}
}

// go test -run Test_Limiter_Next
func Test_Limiter_Next(t *testing.T) {
	app := fiber.New()
	app.Use(New(Config{
		Next: func(_ *fiber.Ctx) bool {
			return true
		},
	}))

	resp, err := app.Test(httptest.NewRequest("GET", "/", nil))
	utils.AssertEqual(t, nil, err)
	utils.AssertEqual(t, fiber.StatusNotFound, resp.StatusCode)
}

func Test_Limiter_Headers(t *testing.T) {
	app := fiber.New()

	app.Use(New(Config{
		Max:      50,
		Duration: 2 * time.Second,
	}))

	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendString("Hello tester!")
	})

	fctx := &fasthttp.RequestCtx{}
	fctx.Request.Header.SetMethod("GET")
	fctx.Request.SetRequestURI("/")
	app.Handler()(fctx)

	utils.AssertEqual(t, "50", string(fctx.Response.Header.Peek("X-RateLimit-Limit")))
	if v := string(fctx.Response.Header.Peek("X-RateLimit-Remaining")); v == "" {
		t.Errorf("The X-RateLimit-Remaining header is not set correctly - value is an empty string.")
	}
	if v := string(fctx.Response.Header.Peek("X-RateLimit-Reset")); !(v == "1" || v == "2") {
		t.Errorf("The X-RateLimit-Reset header is not set correctly - value is out of bounds.")
	}
}

// testStore is used for testing custom stores
type testStore struct {
	stmap map[string][]byte
	mutex *sync.Mutex
}

func (s testStore) Get(id string) ([]byte, error) {
	s.mutex.Lock()
	val, ok := s.stmap[id]
	s.mutex.Unlock()
	if !ok {
		return []byte{}, nil
	} else {
		return val, nil
	}
}

func (s testStore) Set(id string, val []byte, _ time.Duration) error {
	s.mutex.Lock()
	s.stmap[id] = val
	s.mutex.Unlock()

	return nil
}

func (s testStore) Clear() error {
	return nil
}

func (s testStore) Delete(id string) error {
	return nil
}
