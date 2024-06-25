package main

import (
	"log"
	"net/http"

	"github.com/labstack/echo/v5"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/daos"
	"github.com/pocketbase/pocketbase/forms"
	"github.com/pocketbase/pocketbase/models"
)

func bindAppHooks(app core.App) {
	app.OnBeforeServe().Add(func(e *core.ServeEvent) error {
		e.Router.AddRoute(echo.Route{
			Method: http.MethodPost,
			Path:   "/api/ext/collections/modules/records",
			Handler: func(c echo.Context) error {

				data := struct {
					Title  string `json:"title" form:"title"`
					Lesson string `json:"lesson" form:"lesson"`
				}{}

				if err := c.Bind(&data); err != nil {
					return apis.NewBadRequestError("Failed to read request data", err)
				}

				err := app.Dao().RunInTransaction(func(txDao *daos.Dao) error {
					// get the modules collection
					collection, err := txDao.FindCollectionByNameOrId("modules")

					if err != nil {
						return apis.NewApiError(http.StatusInternalServerError, "Failed to find collection", err)
					}

					// get the lesson record
					lessonRecord, err := txDao.FindRecordById("lessons", data.Lesson)

					if err != nil {
						return apis.NewNotFoundError("Lesson not found", err)
					}

					module := models.NewRecord(collection)
					form := forms.NewRecordUpsert(app, module)
					form.SetDao(txDao)
					form.LoadData(map[string]any{"title": data.Title, "lesson": data.Lesson, "content": "hello"})

					if err := form.Submit(); err != nil {
						return apis.NewBadRequestError("Failed to submit form", err)
					}

					// add the module id to the lesson record
					lessonRecord.Set("modules", append(lessonRecord.Get("modules").([]string), module.Id))

					if err := txDao.SaveRecord(lessonRecord); err != nil {
						return err
					}

					return nil
				})

				if err != nil {
					return err
				}
				return c.JSON(http.StatusOK, map[string]string{"title": data.Title, "lesson": data.Lesson, "status": "ok"})
			},
			Middlewares: []echo.MiddlewareFunc{
				apis.RequireRecordAuth(),
				apis.ActivityLogger(app),
			},
		})
		e.Router.AddRoute(echo.Route{
			Method: http.MethodPost,
			Path:   "/api/ext/lessons/:lesson/subscribe",
			Handler: func(c echo.Context) error {
				info := apis.RequestInfo(c)
				record := info.AuthRecord

				if record == nil {
					return apis.NewUnauthorizedError("Unauthorized", nil)
				}

				err := app.Dao().RunInTransaction(func(txDao *daos.Dao) error {

					userLessonCollection, err := txDao.FindCollectionByNameOrId(("user_lessons"))

					if err != nil {
						return err
					}

					userLessonRecord := models.NewRecord(userLessonCollection)
					userLessonRecord.Load(map[string]any{
						"user":   record.Id,
						"lesson": c.PathParam("lesson"),
					})

					// lookup the modules from the lesson
					lessonModules, err := txDao.FindRecordsByExpr("modules", dbx.NewExp("lesson = {:lesson}", dbx.Params{"lesson": c.PathParam("lesson")}))

					if err != nil {
						return err
					}

					// create the user_modules records
					userModuleCollection, err := txDao.FindCollectionByNameOrId("user_modules")

					if err != nil {
						return err
					}

					for _, module := range lessonModules {
						userModuleRecord := models.NewRecord(userModuleCollection)
						userModuleRecord.Load(map[string]any{
							"user":   record.Id,
							"module": module.Id,
						})
						if err := txDao.SaveRecord(userModuleRecord); err != nil {
							return err
						}
					}

					if err := txDao.SaveRecord(userLessonRecord); err != nil {
						return err
					}

					return nil
				})

				if err != nil {
					return err
				}
				return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
			},
			Middlewares: []echo.MiddlewareFunc{
				apis.RequireRecordAuth(),
				apis.ActivityLogger(app),
			},
		})

		return nil
	})
}

func main() {
	app := pocketbase.New()

	bindAppHooks(app)

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}
