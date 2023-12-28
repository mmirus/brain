package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

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

	http.HandleFunc("/", welcome)
	http.HandleFunc("/tasks", tasks)
	http.HandleFunc("/tasks/", task)
	log.Fatal(http.ListenAndServe(":8080", nil))
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

type malformedRequest struct {
	status int
	msg    string
}

func (mr *malformedRequest) Error() string {
	return mr.msg
}

func decodeJsonBody(w http.ResponseWriter, r *http.Request, dst interface{}) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	err := dec.Decode(&dst)
	if err != nil {
		var syntaxError *json.SyntaxError
		var unmarshalTypeError *json.UnmarshalTypeError

		switch {
		case errors.As(err, &syntaxError):
			msg := fmt.Sprintf("Request body contains badly-formed JSON (at position %d)", syntaxError.Offset)
			return &malformedRequest{status: http.StatusBadRequest, msg: msg}

		case errors.As(err, &unmarshalTypeError):
			msg := fmt.Sprintf("Request body contains an invalid value for the %q field (at position %d)", unmarshalTypeError.Field, unmarshalTypeError.Offset)
			return &malformedRequest{status: http.StatusBadRequest, msg: msg}

		case strings.HasPrefix(err.Error(), "json: unknown field "):
			fieldName := strings.TrimPrefix(err.Error(), "json: unknown field ")
			msg := fmt.Sprintf("Request body contains unknown field %s", fieldName)
			return &malformedRequest{status: http.StatusBadRequest, msg: msg}

		case errors.Is(err, io.EOF):
			msg := "Request body must not be empty"
			return &malformedRequest{status: http.StatusBadRequest, msg: msg}

		default:
			return err
		}
	}

	err = dec.Decode(&struct{}{})
	if !errors.Is(err, io.EOF) {
		msg := "Request body must only contain a single JSON object"
		return &malformedRequest{status: http.StatusBadRequest, msg: msg}
	}

	return nil
}
