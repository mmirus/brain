package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type application struct {
	auth struct {
		username string
		password string
	}
}

type Task struct {
	Id        int
	Title     string
	Completed bool
}

type JsonTask struct {
	Id        *int
	Title     *string
	Completed *bool
}

const tasksPath = "tasks"

func main() {
	err := os.Mkdir(tasksPath, 0750)
	if err != nil && !os.IsExist(err) {
		log.Fatal(err)
	}

	err = godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	app := new(application)

	app.auth.username = os.Getenv("AUTH_USERNAME")
	app.auth.password = os.Getenv("AUTH_PASSWORD")

	if app.auth.username == "" {
		log.Fatal("basic auth username must be provided")
	}

	if app.auth.password == "" {
		log.Fatal("basic auth password must be provided")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", app.basicAuth(welcome))
	mux.HandleFunc("/tasks", app.basicAuth(tasks))
	mux.HandleFunc("/tasks/", app.basicAuth(task))

	srv := &http.Server{
		Addr:         ":8080",
		Handler:      mux,
		IdleTimeout:  time.Minute,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	log.Printf("starting server on %s", srv.Addr)
	err = srv.ListenAndServeTLS("./localhost.pem", "./localhost-key.pem")
	log.Fatal(err)
}

func welcome(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Welcome to Brain!")
}

func tasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "POST":
		create(w, r)

	case "GET":
		list(w, r)

	default:
		msg := fmt.Sprintf("Unsupported request method %v to /tasks", r.Method)
		log.Print(msg)
		http.Error(w, msg, http.StatusNotImplemented)
	}
}

func create(w http.ResponseWriter, r *http.Request) {
	var task Task
	err := decodeJsonBody(w, r, &task)
	if err != nil {
		var mr *malformedRequest
		if errors.As(err, &mr) {
			http.Error(w, mr.msg, mr.status)
		} else {
			log.Print(err.Error())
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
		return
	}

	// TODO: secy: validate/sanitize input?

	nextId, err := getNextId()
	if err != nil {
		msg := fmt.Sprintf("An error occurred while saving your task, %q", err.Error())
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}
	task.Id = nextId

	jsonTask, err := json.Marshal(task)
	if err != nil {
		msg := fmt.Sprintf("An error occurred while saving your task, %q", err.Error())
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}

	path := taskPath(task.Id)
	err = os.WriteFile(path, jsonTask, 0644)
	if err != nil {
		msg := fmt.Sprintf("An error occurred while saving your task, %q", err.Error())
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "Task: %+v", task)
}

func getNextId() (int, error) {
	files, err := filepath.Glob(taskPath("*"))

	ids := make([]int, len(files))
	for index, file := range files {
		filename := path.Base(file)
		id, err := strconv.Atoi(strings.Split(filename, ".")[0])
		if err != nil {
			return 0, err
		}
		ids[index] = id
	}
	slices.Sort(ids)

	nextId := 1
	if len(ids) > 0 {
		nextId = ids[len(ids)-1] + 1
	}

	return nextId, err
}

func list(w http.ResponseWriter, r *http.Request) {
	files, err := filepath.Glob(taskPath("*"))
	if err != nil {
		msg := fmt.Sprintf("An error occurred while retrieving tasks, %q", err.Error())
		http.Error(w, msg, http.StatusInternalServerError)
	}

	queryParams := r.URL.Query()
	search := queryParams.Get("q")

	for _, file := range files {
		taskJson, err := os.ReadFile(file)
		if err != nil {
			msg := fmt.Sprintf("An error occurred while retrieving tasks, %q", err.Error())
			http.Error(w, msg, http.StatusInternalServerError)
		}

		if len(search) != 0 {
			var task Task
			json.Unmarshal(taskJson, &task)
			if !strings.Contains(task.Title, search) {
				continue
			}
		}

		fmt.Fprintf(w, string(taskJson))
	}
}

func task(w http.ResponseWriter, r *http.Request) {
	taskId, err := strconv.Atoi(path.Base(r.URL.Path))
	if err != nil {
		msg := fmt.Sprintf("Invalid task ID: %v", path.Base(r.URL.Path))
		http.Error(w, msg, http.StatusBadRequest)
	}

	switch r.Method {
	case "GET":
		show(w, r, taskId)

	case "PUT":
		update(w, r, taskId)

	case "DELETE":
		delete(w, r, taskId)

	default:
		msg := fmt.Sprintf("Unsupported request method %v to /tasks/", r.Method)
		log.Print(msg)
		http.Error(w, msg, http.StatusNotImplemented)
	}
}

func show(w http.ResponseWriter, r *http.Request, taskId int) {
	filename := taskPath(taskId)
	task, err := os.ReadFile(filename)
	if err != nil {
		http.Error(w, "", http.StatusNotFound)
	}
	fmt.Fprintf(w, string(task))
}

func update(w http.ResponseWriter, r *http.Request, taskId int) {
	var taskChanges JsonTask
	err := decodeJsonBody(w, r, &taskChanges)
	if err != nil {
		var mr *malformedRequest
		if errors.As(err, &mr) {
			http.Error(w, mr.msg, mr.status)
		} else {
			log.Print(err.Error())
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
		return
	}

	// TODO: secy: validate/sanitize input?

	filename := taskPath(taskId)
	currentTaskJson, err := os.ReadFile(filename)
	if err != nil {
		http.Error(w, "", http.StatusNotFound)
	}

	var task Task
	json.Unmarshal(currentTaskJson, &task)
	if taskChanges.Title != nil {
		task.Title = *taskChanges.Title
	}
	if taskChanges.Completed != nil {
		task.Completed = *taskChanges.Completed
	}

	updatedTaskJson, err := json.Marshal(task)
	if err != nil {
		msg := fmt.Sprintf("An error occurred while saving your task, %q", err.Error())
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "Task: %+v", task)
	err = os.WriteFile(filename, updatedTaskJson, 0644)
}

func delete(w http.ResponseWriter, r *http.Request, taskId int) {
	filename := taskPath(taskId)
	err := os.Remove(filename)
	if err != nil {
		msg := fmt.Sprintf("An error occurred while deleting task with ID %v: %q", taskId, err.Error())
		http.Error(w, msg, http.StatusInternalServerError)
	}
}

func taskPath(taskId interface{}) string {
	return fmt.Sprintf("%v/%v.json", tasksPath, taskId)
}
