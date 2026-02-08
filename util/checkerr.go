package util

import "fmt"

func CheckErr(err error) {
	if err != nil {
		fmt.Printf("Error occurred: %v\n", err)
		panic(err)
	}
}