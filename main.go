package main

import (
	"crypto/md5"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"html/template"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Film struct {
	Title    string
	Director string
}

type Answer struct {
	Number int
	Text   string
}

type Question struct {
	Order          int64
	QuestionText   string
	Answers        []Answer
	CorrectAnswer  int64
	TotalQuestions int64
}

type Score struct {
	ContestantId   string
	ContestantName string
	Group          string
	CorrectAnswers int64
	TimeTaken      string
}

type Contestant struct {
	ContestantId      string
	ContestantName    string
	QuizId            string
	Group             string
	Started           string
	Finished          string
	CorrectAnswers    int64
	QuestionsAnswered int64
}

var CorrectAnswerText = []string{
	"Well done, you're smarter than you look",
	"Come on, that was a lucky guess wasn't it? I won't tell anyone...",
	"Way to go",
	"Your knowledge is impressive",
	"Even Santa couldn't answer that one!",
}

var IncorrectAnswerText = []string{
	"Better luck with the next one",
	"Rudolph could have answered it",
	"You may get replaced by ChatGPT at this rate...",
	"How did you not know that?!?",
	"You've made the elves cry",
}

func generateContestantId(name string, quiz string, group string) string {
	input := fmt.Sprintf("%s-%s-%s", name, strings.ToLower(quiz), strings.ToLower(group))
	hasher := md5.New()
	hasher.Write([]byte(input))
	hash := hex.EncodeToString(hasher.Sum(nil))
	urlSafeHash := base64.URLEncoding.EncodeToString([]byte(hash))
	return urlSafeHash
}

func getQuizDetails(urlPath string, urlType string) (extractedQuidId string, groupName string) {
	var quidId string
	var group string
	parts := strings.Split(urlPath, "/")
	fmt.Printf("parts %v", parts)

	if len(parts) >= 3 {
		if urlType == "question" {
			quidId = parts[2]
			group = ""
		} else if urlType == "scoreboard" {
			quidId = parts[2]
			group = parts[3]
		} else if urlType == "referrer" {
			quidId = parts[3]
			group = parts[4]
		} else {
			quidId = parts[1]
			group = parts[2]
		}
		fmt.Printf("Quiz name: %s, Group: %s\n", quidId, group)
		return quidId, group
	}

	return
}

func makeDatabaseQuery(query string, args ...interface{}) ([]map[string]interface{}, error) {
	db, err := sql.Open("sqlite3", "./data/quiz-data.db")
	if err != nil {
		fmt.Println("error connecting to database", err)
		return nil, err
	}
	defer db.Close()

	fmt.Printf("Executing query: %s with args: %v\n", query, args)
	resultsFound := false
	rows, err := db.Query(query, args...)
	if err != nil {
		fmt.Println("error in query", err)
		return nil, err
	}
	// fmt.Printf(fmt.Sprintf("rows %v", rows))
	// fmt.Println("\nrow len", rows.Next())
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var result []map[string]interface{}

	for rows.Next() {
		resultsFound = true
		// Create a slice to hold the values of each column
		values := make([]interface{}, len(columns))
		for i := range columns {
			values[i] = new(interface{})
		}

		// Scan the current row into the values slice
		if err := rows.Scan(values...); err != nil {
			return nil, err
		}

		// Create a map to hold the key-value pairs for each column
		rowData := make(map[string]interface{})
		for i, colName := range columns {
			// Use type assertion to extract the actual values from the interface{}
			rowData[colName] = *values[i].(*interface{})
		}

		// Append the map to the result slice
		result = append(result, rowData)
	}

	fmt.Println("rows", resultsFound)

	return result, nil
}

func getQuestionDetails(quizId string, questionNumber string) (string, Question) {
	var quizTitle string = ""
	var retrievedQuestion Question

	questionQuery := `SELECT *, quizzes.name, 
		(SELECT COUNT(*) FROM questions WHERE questions.quiz_id = questions.quiz_id AND active = 1) AS total_questions
		FROM questions 
		LEFT JOIN quizzes ON quizzes.quiz_id = questions.quiz_id
		WHERE questions.quiz_id = ?
		AND questions.sort_order >= ?
		AND questions.active = 1`

	result, err := makeDatabaseQuery(questionQuery, quizId, questionNumber)
	if err != nil {
		fmt.Println("Error getting quiz details", err.Error())
		log.Fatal(err)
	}

	if len(result) > 0 {
		quizTitle = result[0]["name"].(string)
		retrievedQuestion = Question{
			Order:        result[0]["sort_order"].(int64),
			QuestionText: result[0]["question"].(string),
			Answers: []Answer{
				{
					Number: 1,
					Text:   result[0]["answer_1"].(string),
				},
				{
					Number: 2,
					Text:   result[0]["answer_2"].(string),
				},
				{
					Number: 3,
					Text:   result[0]["answer_3"].(string),
				},
				{
					Number: 4,
					Text:   result[0]["answer_4"].(string),
				},
			},
			CorrectAnswer:  result[0]["correct_answer"].(int64),
			TotalQuestions: result[0]["total_questions"].(int64),
		}
	}

	return quizTitle, retrievedQuestion
}

func insertContestant(quizId string, contestantName string, group string) string {
	var contestantId int64

	db, err := sql.Open("sqlite3", "./data/quiz-data.db")
	if err != nil {
		fmt.Println("error connecting to database", err.Error())
		return ""
	}
	defer db.Close()

	generatedContestantId := generateContestantId(contestantName, quizId, group)
	insertQuery := "INSERT INTO scores(quiz_id, `group`, name, correct_answers, questions_answered, contestant_id) VALUES (?, ?, ?, 0, 0, ?)"
	fmt.Printf("Executing query: %s with args: %s %s %s %s\n", insertQuery, quizId, strings.ToLower(group), contestantName, generatedContestantId)

	insertedId, err := db.Exec(insertQuery, quizId, strings.ToLower(group), contestantName, generatedContestantId)
	if err != nil {
		fmt.Println("error in query", err.Error())
		return ""
	}

	contestantId, err = insertedId.LastInsertId()
	if err != nil {
		fmt.Println("error getting last insert ID", err.Error())
		return ""
	}
	fmt.Println("Inserted ID", contestantId)

	return generatedContestantId
}

func createContestant(quizId string, contestantName string, group string) string {
	var contestantId string

	// we're only interested in the person has started the quiz, if a person with the same name registers again they can if they haven't started
	checkExistsQuery := "SELECT contestant_id, questions_answered FROM scores WHERE quiz_id = ? AND `group` = ? AND name = ?"
	existsResult, existsErr := makeDatabaseQuery(checkExistsQuery, strings.ToLower(quizId), strings.ToLower(group), contestantName)
	if existsErr != nil {
		fmt.Println("Error creating person record", existsErr.Error())
		log.Fatal(existsErr.Error())
	}

	if len(existsResult) > 0 {
		fmt.Println("Found existing record")
		// we found a record with those details already
		if existsResult[0]["questions_answered"].(int64) < 1 {
			// if they haven't actually answered anything, continue with the retrieved ID
			return existsResult[0]["contestant_id"].(string)
		} else {
			// if they have already answered questions prevent them starting again
			return ""
		}
	}

	// if no record was found, insert the record and return the ID
	contestantId = insertContestant(quizId, contestantName, group)

	return contestantId
}

func getContestantDetails(contestantId string) Contestant {
	var contestantDetails Contestant

	contestantQuery := "SELECT * FROM scores WHERE contestant_id = ?"
	contestantResult, err := makeDatabaseQuery(contestantQuery, contestantId)
	if err != nil {
		fmt.Println("Error retrieving contestant details", err.Error())
	}

	if len(contestantResult) > 0 {
		contestantDetails.ContestantId = contestantId
		contestantDetails.ContestantName = contestantResult[0]["name"].(string)
		contestantDetails.Group = contestantResult[0]["group"].(string)
		if contestantResult[0]["started"] == nil {
			contestantDetails.Started = ""
		} else {
			contestantDetails.Started = contestantResult[0]["started"].(string)
		}
		if contestantResult[0]["finished"] == nil {
			contestantDetails.Started = ""
		} else {
			contestantDetails.Finished = contestantResult[0]["finished"].(string)
		}
		contestantDetails.QuizId = contestantResult[0]["quiz_id"].(string)
		contestantDetails.CorrectAnswers = contestantResult[0]["correct_answers"].(int64)
		contestantDetails.QuestionsAnswered = contestantResult[0]["questions_answered"].(int64)
	}

	return contestantDetails
}

func updateContestant(contestantId string, setStarted bool, correctAnswer bool) bool {

	updateQuery := "UPDATE scores SET questions_answered = questions_answered + 1 WHERE contestant_id = ?"
	if setStarted {
		updateQuery = "UPDATE scores SET started = DATETIME('now') WHERE contestant_id = ?"
	}
	if correctAnswer {
		updateQuery = "UPDATE scores SET correct_answers = correct_answers + 1, questions_answered = questions_answered + 1 WHERE contestant_id = ?"
	}
	updateResult, updateErr := makeDatabaseQuery(updateQuery, contestantId)
	if updateErr != nil {
		fmt.Println("Error updating score for", contestantId, updateErr.Error())
		log.Fatal(updateErr.Error())
	}

	if updateResult != nil || len(updateResult) > 0 {
		return false
	}

	return true
}

func secondsToDurationString(durationInSeconds int64) string {
	duration := time.Second * time.Duration(durationInSeconds)

	hours := int(duration.Hours())
	minutes := int(duration.Minutes()) % 60
	seconds := int(duration.Seconds()) % 60

	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
}

func getGroupScores(quizId string, group string) []Score {
	groupScoreQuery := `SELECT contestant_id, name, correct_answers, 
		(strftime('%s', finished) - strftime('%s', started)) AS time_taken_seconds
		FROM scores
		WHERE quiz_id = ?
		AND "group" = ?
		ORDER BY correct_answers DESC, (strftime('%s', finished) - strftime('%s', started)) ASC`
	groupScoreResult, err := makeDatabaseQuery(groupScoreQuery, quizId, group)
	if err != nil {
		fmt.Println("Error getting scores", err.Error())
	}

	var scores []Score

	for key, row := range groupScoreResult {
		fmt.Println("row:", key)
		formattedTimeTaken := secondsToDurationString(row["time_taken_seconds"].(int64))

		thisScore := Score{
			ContestantId:   row["contestant_id"].(string),
			ContestantName: row["name"].(string),
			Group:          group,
			CorrectAnswers: row["correct_answers"].(int64),
			TimeTaken:      formattedTimeTaken,
		}
		scores = append(scores, thisScore)
	}

	return scores
}

func main() {

	home := func(w http.ResponseWriter, r *http.Request) {
		// io.WriteString(w, r.Method)
		fmt.Println(r.URL)

		var quizName string
		var group string
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) >= 3 {
			quizName = parts[1]
			group = parts[2]
			fmt.Printf("Quiz name: %s, Group: %s\n", quizName, group)
		} else {
			// change to send back an error message based on what is missing from the URL
			http.NotFound(w, r)
		}

		tmpl := template.Must(template.ParseFiles("templates/home.html"))
		films := map[string]interface{}{
			"Films": []Film{
				{Title: "Jurassic Park", Director: "Steven Spielberg"},
				{Title: "Star Wars", Director: "George Lucas"},
				{Title: "Ghostbusters", Director: "Ivan Reitman"},
			},
		}

		if quizName != "" {
			var questions []Question
			db, err := sql.Open("sqlite3", "./data/quiz-data.db")
			if err != nil {
				fmt.Println("error connecting to database", err)
				log.Fatal(err)
			}
			defer db.Close()

			quizQuery := `SELECT 
			sort_order, 
			question, 
			answer_1, 
			answer_2, 
			answer_3, 
			answer_4, 
			correct_answer 
			FROM questions 
			WHERE quiz_id = ?
			AND active = 1`

			rows, err := db.Query(quizQuery, quizName)
			if err != nil {
				fmt.Println("error in query", err)
				log.Fatal(err)
			}
			defer rows.Close()

			for rows.Next() {
				var order, correct_answer int
				var question string
				var answer_1, answer_2, answer_3, answer_4 sql.NullString
				err := rows.Scan(&order, &question, &answer_1, &answer_2, &answer_3, &answer_4, &correct_answer)
				if err != nil {
					log.Fatal(err)
				}
				fmt.Printf("question: %d is: %s and the correct answer is: %d\n", order, question, correct_answer)
				// questionDetails := Question{
				// 	Order:         order,
				// 	Question:      question,
				// 	Answers:       []sql.NullString{answer_1, answer_2, answer_3, answer_4},
				// 	CorrectAnswer: correct_answer,
				// }
				// questions = append(questions, questionDetails)
			}

			// Check for errors from iterating over rows
			if err := rows.Err(); err != nil {
				log.Fatal(err)
			}

			films["Questions"] = questions
		}

		tmpl.Execute(w, films)
	}

	start := func(w http.ResponseWriter, r *http.Request) {
		quizId, group := getQuizDetails(r.URL.Path, "initial")
		fmt.Println(group)
		quizTitle := "Not Found"

		if quizId != "" {
			quizDetails := "SELECT name FROM quizzes WHERE quiz_id = ?"
			result, err := makeDatabaseQuery(quizDetails, quizId)
			if err != nil {
				fmt.Println("Error getting quiz details", err)
				log.Fatal(err)
			}
			if len(result) > 0 {
				// for _, row := range result {
				// 	fmt.Println(row["name"])
				// }
				quizTitle = result[0]["name"].(string)
				fmt.Println("quiz title", quizTitle)
			}
			// quizInfo, err := json.Marshal(map[string]string{"name": quizTitle})
			// if err != nil {
			// 	fmt.Println(err.Error())
			// }
			// fmt.Println("quizInfo", string(quizInfo))
			// http.SetCookie(w, &http.Cookie{Name: "quizDetails", Value: string(quizInfo), Path: "/"})
		}

		// tmpl := template.Must(template.ParseFiles("templates/home.html"))
		tmpl, err := template.ParseFiles("templates/base.html", "templates/home.html")
		// tmpl, err := template.ParseFiles("templates/home.html.bkcp")
		if err != nil {
			fmt.Println("Error rendering template", err.Error())
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		err = tmpl.ExecuteTemplate(w, "base", map[string]string{
			"QuizTitle": quizTitle,
			"QuizId":    quizId,
			"Group":     group,
		})
		if err != nil {
			fmt.Println(err.Error())
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	}

	quiz := func(w http.ResponseWriter, r *http.Request) {
		// modify this to look for a supplied question number
		questionNum := 1
		quizStarted := false
		currentQuestion := r.PostFormValue("question")
		contestantId := r.PostFormValue("contestant-id")
		fmt.Println("Contestant ID", contestantId)
		quizId, group := getQuizDetails(r.URL.Path, "question")

		// if no contestant-id was supplied in the form submit, we assume this is the first question, which is the first page after the start
		// so we need to create contestant details
		if contestantId == "" {
			fmt.Println(r.Referer())
			quizId, group = getQuizDetails(r.Referer(), "referrer")
			contestantName := r.PostFormValue("contestant-name")
			contestantId = createContestant(quizId, contestantName, group)
		}

		contestantDetails := getContestantDetails(contestantId)
		// group := r.PostFormValue("group")
		fmt.Println("group", contestantDetails.Group, "vs", group)
		if len(currentQuestion) > 0 {
			// add one to get the next question
			convertedNum, _ := strconv.Atoi(currentQuestion)
			questionNum = convertedNum + 1
			quizStarted = true
		}
		// use that in the query, if not found assume first one
		// fmt.Println(r.Referer())
		// contestantName := r.PostFormValue("contestant-name")
		// w.Header().Add("X-contestant-name", contestantName)
		// c, _ := r.Cookie("quizDetails")
		// fmt.Println("cookie", c)

		var quizTitle string
		var retrievedQuestion Question

		if quizId != "" {
			quizTitle, retrievedQuestion = getQuestionDetails(quizId, strconv.Itoa(questionNum))
		}

		templatesToRender := []string{
			"templates/base.html",
			"templates/quiz.html",
			"templates/question.html",
		}
		// if this isn't the first question we only need the question element rendered
		if questionNum != 1 || quizStarted {
			templatesToRender = []string{
				"templates/question.html",
			}
		} else {
			updateSucceeded := updateContestant(contestantId, true, false)
			if !updateSucceeded {
				fmt.Println("Error when setting started datetime")
			}
		}

		// if there was an existing record found
		if contestantId == "" {
			templatesToRender = []string{
				"templates/base.html",
				"templates/home.html",
			}
		}

		tmpl, err := template.ParseFiles(templatesToRender...)
		if err != nil {
			fmt.Println("Error rendering template", err.Error())
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		templateValues := map[string]interface{}{
			"QuizTitle":  quizTitle,
			"QuizId":     quizId,
			"Question":   retrievedQuestion,
			"Contestant": contestantId,
			"Group":      contestantDetails.Group,
		}

		if contestantId == "" {
			templateValues = map[string]interface{}{
				"QuizTitle":       quizTitle,
				"QuizId":          quizId,
				"Group":           contestantDetails.Group,
				"ExistingMessage": true,
			}
		}

		if questionNum != 1 || quizStarted {
			err = tmpl.ExecuteTemplate(w, "question", templateValues)
		} else {
			err = tmpl.ExecuteTemplate(w, "base", templateValues)
		}
		if err != nil {
			fmt.Println(err.Error())
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}

	}

	recordAnswer := func(w http.ResponseWriter, r *http.Request) {
		// get the submitted form details
		var gradeText string
		rand.Seed(time.Now().UnixNano())
		// Generate a random number between 0 and 5
		randomNumber := rand.Intn(6)
		questionAnswered := r.PostFormValue("question")
		questionAnsweredInt, _ := strconv.Atoi(questionAnswered)
		contestantId := r.PostFormValue("contestant-id")
		contestantDetails := getContestantDetails(contestantId)
		selectedAnswer := r.PostFormValue("answers")
		selectedAnswerInt, err := strconv.Atoi(selectedAnswer)
		if err != nil {
			fmt.Println("Error converting answer string to int", err.Error())
		}
		if err == nil {
			// check if this is the correct answer
			// correctAnswerQuery := `SELECT correct_answer FROM questions WHERE quiz_id = ? AND sort_order = ?`
			// result, err := makeDatabaseQuery(correctAnswerQuery, quizId, questionAnswered)
			// if err != nil {
			// 	fmt.Println("Error retrieving answer", err.Error())
			// }
			var retrievedQuestion Question
			_, retrievedQuestion = getQuestionDetails(contestantDetails.QuizId, questionAnswered)
			fmt.Println("correct answer", retrievedQuestion.CorrectAnswer)

			if retrievedQuestion.CorrectAnswer != 0 {

				if selectedAnswerInt == int(retrievedQuestion.CorrectAnswer) {
					// update the score if this is the correct answer
					fmt.Println("correct answer selected")
					gradeText = fmt.Sprintf("<span class=\"green\">Correct!<span> %s", CorrectAnswerText[randomNumber])
					updateSucceeded := updateContestant(contestantId, false, true)
					if !updateSucceeded {
						fmt.Println("Error when updating answer totals")
					}
				} else {
					gradeText = fmt.Sprintf("<span class=\"error\">Incorrect!<span> %s", IncorrectAnswerText[randomNumber])
					updateSucceeded := updateContestant(contestantId, false, false)
					if !updateSucceeded {
						fmt.Println("Error when updating answer totals")
					}
				}

				// if this is the last question, set the finish time
				if questionAnsweredInt == int(retrievedQuestion.TotalQuestions) {
					finishQuery := "UPDATE scores SET finished = DATETIME('now') WHERE contestant_id = ?"
					_, err := makeDatabaseQuery(finishQuery, contestantId)
					if err != nil {
						fmt.Println("Error setting finish time", err.Error())
					}
				}

				// return the answer
				// include a next button to move to the next one
				tmpl, err := template.ParseFiles("templates/question.html")
				if err != nil {
					fmt.Println("Error rendering template", err.Error())
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
					return
				}

				templateValues := map[string]interface{}{
					"QuizId":     contestantDetails.QuizId,
					"Question":   retrievedQuestion,
					"Contestant": contestantId,
					"Answer":     true,
					"GradeText":  template.HTML(gradeText),
				}

				err = tmpl.ExecuteTemplate(w, "question", templateValues)
				if err != nil {
					fmt.Println(err.Error())
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
			}
		}
	}

	scoreboard := func(w http.ResponseWriter, r *http.Request) {
		quizId, group := getQuizDetails(r.URL.Path, "scoreboard")
		fmt.Println(group)
		quizTitle := "Not Found"
		totalQuestions := int64(0)

		if quizId != "" {
			quizDetails := "SELECT name, (SELECT COUNT(*) FROM questions WHERE quiz_id = ? AND active = 1) AS total_questions FROM quizzes WHERE quiz_id = ?"
			result, err := makeDatabaseQuery(quizDetails, quizId, quizId)
			if err != nil {
				fmt.Println("Error getting quiz details", err)
				log.Fatal(err)
			}
			if len(result) > 0 {
				quizTitle = result[0]["name"].(string)
				totalQuestions = result[0]["total_questions"].(int64)
				fmt.Println("quiz title", quizTitle)
			}
		}

		var contestantId string
		if r.URL.RawQuery != "" {
			q := r.URL.Query()
			contestantId = q.Get("c")
		} else {
			// if the URL doesn't contain a contestant ID, check if this was a form submit (i.e. the last question)
			if r.Method == "POST" {
				// if so, it should include the contestant ID
				contestantId = r.PostFormValue("contestant-id")
			}
		}

		var groupScores []Score
		templatesToRender := []string{
			"templates/base.html",
			"templates/scoreboard.html",
		}
		showError := true

		var contestantDetails Contestant
		if contestantId != "" {
			contestantDetails = getContestantDetails(contestantId)

			// get all scores for the group, sort by points and total time
			groupScores = getGroupScores(quizId, contestantDetails.Group)

			showError = false

		}

		tmpl, err := template.ParseFiles(templatesToRender...)
		if err != nil {
			fmt.Println("Error rendering template", err.Error())
		}

		err = tmpl.ExecuteTemplate(w, "base", map[string]interface{}{
			"QuizTitle":      quizTitle,
			"TotalQuestions": totalQuestions,
			"Scores":         groupScores,
			"Contestant":     contestantDetails,
			"ShowError":      showError,
		})
		if err != nil {
			fmt.Println(err.Error())
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	}

	addFilm := func(w http.ResponseWriter, r *http.Request) {
		// log.Print("HTMX request received")
		// log.Print(r.Header.Get("HX-Request"))

		title := r.PostFormValue("title")
		director := r.PostFormValue("director")

		fmt.Println(title)
		fmt.Println(director)

		// htmlStr := fmt.Sprintf("<li>%s <i>dir.</i> %s</li>", title, director)
		// tmpl, _ := template.New("t").Parse(htmlStr)
		// tmpl.Execute(w, nil)
		tmpl := template.Must(template.ParseFiles("templates/home.html"))
		tmpl.ExecuteTemplate(w, "film-list-element", Film{Title: title, Director: director})
	}

	// http.HandleFunc("/", home)
	http.HandleFunc("/quiz/", quiz)
	http.HandleFunc("/record-answer/", recordAnswer)
	http.HandleFunc("/scoreboard/", scoreboard)
	http.HandleFunc("/", start)
	http.HandleFunc("/add-film/", addFilm)
	http.HandleFunc("/ignore/", home)

	// port := os.Getenv("PORT")
	// if port == "" {
	// 	port = "8080"
	// }
	port := "8001"
	fmt.Println("Starting server on port", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}