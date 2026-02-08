package getinfo

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/cert-manager/cert-manager/pkg/util"
)

func GetInfo(w http.ResponseWriter, r *http.Request) {
	db := util.LoginToDB()

	type idata struct {
		ID  int
		TITLE string
		ABOUT string
		DATE string
	}

	var id int
	var title string
	var about string
	var date string
	var datarows []idata
	rows, err := db.Query("SELECT * FROM info")
	util.CheckErr(err)
	for rows.Next() {
		switch err := rows.Scan(&id, &title, &about, &date); err {
			case sql.ErrNoRows:
				fmt.Println("No rows were returned!")
			case nil:
				datarows = append(datarows, idata{
					ID: id, 
					TITLE: title, 
					ABOUT: about, 
					DATE: date,
				})
			default:
				util.CheckErr(err)
		}
	}

	jsoninfo, _ := json.Marshal(datarows)
	io.WriteString(w, string(jsoninfo))
}