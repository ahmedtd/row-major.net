package wordgrid

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"go.opentelemetry.io/otel"
)

type Handler struct {
	evaluator *Evaluator
}

func NewHandler(e *Evaluator) *Handler {
	return &Handler{
		evaluator: e,
	}
}

func NewHandlerFromFile(wordsFile string) (*Handler, error) {
	ev, err := NewFromFile(wordsFile)
	if err != nil {
		return nil, err
	}
	return NewHandler(ev), nil
}

type solveRequest struct {
	Constraints []sparseConstraint
}

type sparseConstraint struct {
	Row    int
	Col    int
	Letter string
}

type solveResponse struct {
	FoundSolution  bool
	Grid           []string
	ConstraintGrid []string
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tracer := otel.Tracer("row-major/rumor-mill/wordgrid")
	ctx, span := tracer.Start(r.Context(), "WordGrid Serve HTTP")
	defer span.End()

	reqBody, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	req := &solveRequest{}
	if err := json.Unmarshal(reqBody, req); err != nil {
		http.Error(w, fmt.Sprintf("bad request: invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	constraints := make([]rune, 5*5)
	constraintStrings := make([]string, 5*5)
	for _, sc := range req.Constraints {
		if len([]rune(sc.Letter)) != 1 {
			http.Error(w, "bad request: constraint letters must be single characters", http.StatusBadRequest)
			return
		}
		constraints[sc.Row*5+sc.Col] = []rune(sc.Letter)[0]
		constraintStrings[sc.Row*5+sc.Col] = string(constraints[sc.Row*5+sc.Col])
	}

	w.Header().Add("Content-Type", "application/json; charset=utf-8")

	response := &solveResponse{
		FoundSolution:  false,
		Grid:           make([]string, 5*5),
		ConstraintGrid: constraintStrings,
	}
	e := h.evaluator.SubEvaluator(constraints)
	ok := e.Search(ctx)
	if ok {
		response.FoundSolution = true
		response.Grid = e.SolutionAsGrid()
	}

	respBody, err := json.Marshal(response)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Write(respBody)
}
