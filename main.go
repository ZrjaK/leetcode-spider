package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
	"github.com/spf13/cast"

	"github.com/tidwall/gjson"
	"xorm.io/xorm"
)

var (
	Cookie     string
	DataSource string
)

var engine *xorm.Engine

type SpecialQuestion struct {
	QuestionId      string    `xorm:"pk"`
	Title           string    `xorm:""`
	TranslatedTitle string    `xorm:""`
	TitleSlug       string    `xorm:""`
	Difficulty      string    `xorm:""`
	NumSubmitted    int64     `xorm:""`
	LastSubmittedAt time.Time `xorm:""`
}

type Question struct {
	QuestionId      int64     `xorm:"pk"`
	Title           string    `xorm:""`
	TranslatedTitle string    `xorm:""`
	TitleSlug       string    `xorm:""`
	Difficulty      string    `xorm:""`
	NumSubmitted    int64     `xorm:""`
	LastSubmittedAt time.Time `xorm:""`
}

type LastSubmission struct {
	DetailId   int64  `xorm:"pk"`
	QuestionId string `xorm:""`
	Language   string `xorm:""`
	Code       string `xorm:"LONGTEXT"`
}

var client = http.Client{}

func main() {
	InitEnv()
	InitXormEngine()
	UpdateAcceptedQuestion()
	GenerateFile()
}

func GenerateFile() {
	engine.Iterate(new(LastSubmission), func(i int, bean interface{}) error {
		sub := bean.(*LastSubmission)
		c := "//"
		var ext string
		switch sub.Language {
		case "python3":
			c = "#"
			ext = ".py"
		case "golang":
			ext = ".go"
		case "cpp":
			ext = ".cpp"
		case "java":
			ext = ".java"
		}
		text := ""
		question := Question{}
		exist, _ := engine.Table("question").Where("question_id = ?", sub.QuestionId).Get(&question)
		if exist {
			text += fmt.Sprintf("%v 题目：%v.%v\n", c, question.QuestionId, question.TranslatedTitle)
			text += fmt.Sprintf("%v 难度：%v\n", c, question.Difficulty)
			text += fmt.Sprintf("%v 最后提交：%v\n", c, question.LastSubmittedAt)
			text += fmt.Sprintf("%v 语言：%v\n", c, sub.Language)
			text += fmt.Sprintf("%v 作者：ZrjaK\n\n", c)
			text += sub.Code
			os.Mkdir("answer", 0666)
			if question.TranslatedTitle == "" {
				return errors.New("查询题目出错")
			}
			ioutil.WriteFile(path.Join("answer", fmt.Sprintf("%v.%v%v", question.QuestionId, question.TranslatedTitle, ext)),
				[]byte(text), 0666)
		} else {
			sp := SpecialQuestion{}
			engine.Table("special_question").Where("question_id = ?", sub.QuestionId).Get(&sp)
			text += fmt.Sprintf("%v 题目：%v.%v\n", c, sp.QuestionId, sp.TranslatedTitle)
			text += fmt.Sprintf("%v 难度：%v\n", c, sp.Difficulty)
			text += fmt.Sprintf("%v 最后提交：%v\n", c, sp.LastSubmittedAt)
			text += fmt.Sprintf("%v 语言：%v\n", c, sub.Language)
			text += fmt.Sprintf("%v 作者：ZrjaK\n\n", c)
			text += sub.Code
			os.Mkdir("answer", 0666)
			if sp.TranslatedTitle == "" {
				return errors.New("查询题目出错")
			}
			ioutil.WriteFile(path.Join("answer", fmt.Sprintf("%v.%v%v", sp.QuestionId, sp.TranslatedTitle, ext)),
				[]byte(text), 0666)
		}
		return nil
	})
}

func GetSubmissionList(questionSlug string) LastSubmission {
	req, _ := http.NewRequest(`POST`, `https://leetcode.cn/graphql/`, strings.NewReader(fmt.Sprintf(`{
		"operationName": "progressSubmissions",
		"variables": {
		  "offset": 0,
		  "limit": 10,
		  "questionSlug": "%v"
		},
		"query": "query progressSubmissions($offset: Int, $limit: Int, $lastKey: String, $questionSlug: String) {\n  submissionList(offset: $offset, limit: $limit, lastKey: $lastKey, questionSlug: $questionSlug) {\n    lastKey\n    hasNext\n    submissions {\n      id\n      timestamp\n      url\n      lang\n      runtime\n      __typename\n    }\n    __typename\n  }\n}\n"
	  }`, questionSlug)))
	req.Header.Set("Cookie", Cookie)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/104.0.0.0 Safari/537.36")
	req.Header.Set("Host", "leetcode.cn")
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		fmt.Println(err)
	}
	defer res.Body.Close()
	body, _ := ioutil.ReadAll(res.Body)
	submissions := gjson.GetBytes(body, "data.submissionList.submissions")
	var lastSubmission LastSubmission
	for _, sub := range submissions.Array() {
		if sub.Get("runtime").String() != "N/A" {
			lastSubmission = GetLastSubmissionDetail(sub.Get("id").String())
			lastSubmission.DetailId = sub.Get("id").Int()
			break
		}
	}
	return lastSubmission
}

func GetLastSubmissionDetail(id string) LastSubmission {
	req, _ := http.NewRequest(`GET`, `https://leetcode.cn/graphql/`, strings.NewReader(fmt.Sprintf(`{
		"operationName": "mySubmissionDetail",
		"variables": {
		  "id": "%v"
		},
		"query": "query mySubmissionDetail($id: ID!) {\n  submissionDetail(submissionId: $id) {\n    id\n    code\n    runtime\n    memory\n    rawMemory\n    statusDisplay\n    timestamp\n    lang\n    isMine\n    passedTestCaseCnt\n    totalTestCaseCnt\n    sourceUrl\n    question {\n      titleSlug\n      title\n      translatedTitle\n      questionId\n      __typename\n    }\n    ... on GeneralSubmissionNode {\n      outputDetail {\n        codeOutput\n        expectedOutput\n        input\n        compileError\n        runtimeError\n        lastTestcase\n        __typename\n      }\n      __typename\n    }\n    submissionComment {\n      comment\n      flagType\n      __typename\n    }\n    __typename\n  }\n}\n"
	  }`, id)))
	req.Header.Set("Cookie", Cookie)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/104.0.0.0 Safari/537.36")
	req.Header.Set("Host", "leetcode.cn")
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		fmt.Println(err)
	}
	defer res.Body.Close()
	body, _ := ioutil.ReadAll(res.Body)
	lastSubmission := LastSubmission{}
	detail := gjson.GetBytes(body, "data.submissionDetail")
	lastSubmission.Code = detail.Get("code").String()
	lastSubmission.Language = detail.Get("lang").String()
	return lastSubmission
}

func UpdateAcceptedQuestion() {
	req, _ := http.NewRequest(`POST`, `https://leetcode.cn/graphql/`, strings.NewReader(`{
		"operationName": "userProfileQuestions",
		"variables": {
		  "status": "ACCEPTED",
		  "skip": 0,
		  "first": 10000,
		  "sortField": "LAST_SUBMITTED_AT",
		  "sortOrder": "DESCENDING",
		  "difficulty": []
		},
		"query": "query userProfileQuestions($status: StatusFilterEnum!, $skip: Int!, $first: Int!, $sortField: SortFieldEnum!, $sortOrder: SortingOrderEnum!, $keyword: String, $difficulty: [DifficultyEnum!]) {\n  userProfileQuestions(status: $status, skip: $skip, first: $first, sortField: $sortField, sortOrder: $sortOrder, keyword: $keyword, difficulty: $difficulty) {\n    totalNum\n    questions {\n      translatedTitle\n      frontendId\n      titleSlug\n      title\n      difficulty\n      lastSubmittedAt\n      numSubmitted\n      lastSubmissionSrc {\n        sourceType\n        ... on SubmissionSrcLeetbookNode {\n          slug\n          title\n          pageId\n          __typename\n        }\n        __typename\n      }\n      __typename\n    }\n    __typename\n  }\n}\n"
	  }
	  `))
	req.Header.Set("Cookie", Cookie)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/104.0.0.0 Safari/537.36")
	req.Header.Set("Host", "leetcode.cn")
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		fmt.Println(err)
	}
	defer res.Body.Close()
	body, _ := ioutil.ReadAll(res.Body)
	q, sp := GetAcceptedQuestion(body)
	engine.Sync(new(Question))
	engine.Where(`1 = 1`).Delete(new(Question))
	engine.Insert(&q)

	engine.Sync(new(SpecialQuestion))
	engine.Where(`1 = 1`).Delete(new(SpecialQuestion))
	engine.Insert(&sp)

	engine.Sync(new(LastSubmission))
	for h, i := range q {
		n, _ := engine.Table("last_submission").Where("question_id = ?", i.QuestionId).Count()
		if n > 0 {
			continue
		}
		lastSubmission := GetSubmissionList(i.TitleSlug)
		lastSubmission.QuestionId = cast.ToString(i.QuestionId)
		if lastSubmission.Code == "" {
			fmt.Println(h, i.QuestionId, "--------------")
			continue
		}
		engine.Insert(&lastSubmission)
		engine.Update(&lastSubmission)
		time.Sleep(time.Millisecond * 100)
		fmt.Println(h, i.QuestionId, lastSubmission.DetailId, lastSubmission.Language)
	}
	for _, i := range sp {
		n, _ := engine.Where("question_id = ?", i.QuestionId).Count()
		if n > 0 {
			continue
		}
		lastSubmission := GetSubmissionList(i.TitleSlug)
		lastSubmission.QuestionId = i.QuestionId
		engine.Insert(&lastSubmission)
		engine.Update(&lastSubmission)
		time.Sleep(time.Millisecond * 100)
	}
}

func GetAcceptedQuestion(j []byte) ([]Question, []SpecialQuestion) {
	questionList := []Question{}
	specialQuestionList := []SpecialQuestion{}
	questions := gjson.GetBytes(j, "data.userProfileQuestions.questions")
	for _, q := range questions.Array() {
		_, err := strconv.Atoi(q.Get("frontendId").String())
		if err == nil {
			questionList = append(questionList, Question{
				TranslatedTitle: q.Get("translatedTitle").String(),
				QuestionId:      q.Get("frontendId").Int(),
				TitleSlug:       q.Get("titleSlug").String(),
				Title:           q.Get("title").String(),
				Difficulty:      q.Get("difficulty").String(),
				LastSubmittedAt: time.Unix(q.Get("lastSubmittedAt").Int(), 0),
				NumSubmitted:    q.Get("numSubmitted").Int(),
			})
		} else {
			specialQuestionList = append(specialQuestionList, SpecialQuestion{
				TranslatedTitle: q.Get("translatedTitle").String(),
				QuestionId:      q.Get("frontendId").String(),
				TitleSlug:       q.Get("titleSlug").String(),
				Title:           q.Get("title").String(),
				Difficulty:      q.Get("difficulty").String(),
				LastSubmittedAt: time.Unix(q.Get("lastSubmittedAt").Int(), 0),
				NumSubmitted:    q.Get("numSubmitted").Int(),
			})
		}
	}
	return questionList, specialQuestionList
}

func InitEnv() {
	if err := godotenv.Load(".env"); err == nil {
		Cookie = os.Getenv("Cookie")
		DataSource = os.Getenv("DataSource")
	}
}

func InitXormEngine() {
	var err error
	if engine, err = xorm.NewEngine("mysql", DataSource); err != nil {
		log.Fatal(err)
	}
}
