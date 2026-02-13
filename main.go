package main

import (
	"context"
	"log"
	"fmt"
	"net/http"
	"os" // 環境変数を読み込むために追加


	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"github.com/improbable-eng/grpc-web/go/grpcweb"
	"gorm.io/driver/mysql" // MariaDB用
	"gorm.io/gorm"

	"auxilia/pb"
)

func handler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "Hello, Go Server! Unity can connect here.")
}

func main() {
	http.HandleFunc("/", handler)

	// ポート番号を環境変数から取得（なければ8080を使う）
	// クラウドサーバーにデプロイした際、自動で割り振られるポートに対応するため
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Printf("Server starting at http://localhost:%s\n", port)
	
	// ":8080" のように指定
	err := http.ListenAndServe(":"+port, nil)
	if err != nil {
		fmt.Printf("Failed to start server: %v\n", err)
	}
}