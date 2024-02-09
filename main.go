package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"io"
	"log"
	"net/http"
	"net/url"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type tokenReq struct {
	SystemName string `json:"systemName"`
}

type createPaymentParameter struct {
	SystemName   string  `json:"systemName"`
	TokenApi     string  `json:"tokenApi"`
	GenQrApi     string  `json:"genQrApi"`
	Amount       float64 `json:"amount"`
	CustomerNo   string  `json:"customerNo"`
	CustomerUser string  `json:"customerUser"`
	Branch       string  `json:"branch"`
	Suffix       int     `json:"suffix"`
	DotTaxId     string  `json:"dotTaxId"`
	Description  string  `json:"description"`
	ExpireDate   string  `json:"expireDate"`
	InvoiceDate  string  `json:"invoiceDate"`
	Reference1   string  `json:"ref1"`
	Reference2   string  `json:"ref2"`
}

type genQrParameter struct {
	SystemName   string  `json:"systemName"`
	Amount       float64 `json:"amount"`
	CustomerNo   string  `json:"customerNo"`
	CustomerUser string  `json:"customerUser"`
	Branch       string  `json:"branch"`
	Suffix       int     `json:"suffix"`
	DotTaxId     string  `json:"dotTaxId"`
	Description  string  `json:"description"`
	ExpireDate   string  `json:"expireDate"`
	InvoiceDate  string  `json:"invoiceDate"`
	Reference1   string  `json:"ref1"`
	Reference2   string  `json:"ref2"`
}

type genQrRes struct {
	RedirectUrl    string `json:"redirectUrl"`
	ResultCode     int    `json:"resultCode"`
	ResultDesc     string `json:"resultDesc"`
	DevelopMessage string `json:"developMessage"`
}

type accessRes struct {
	AccessToken string `json:"accessToken"`
	Expires     int    `json:"expires"`
}

type CallbackParameter struct {
	ResponseCode    int    `json:"responseCode"`
	ResponseMessage string `json:"responseMsg"`
	TransactionId   string `json:"transactionId"`
	Reference1      string `json:"ref1"`
	Reference2      string `json:"ref2"`
}

type WebSocketClient struct {
	conn      *websocket.Conn
	ClientRef string
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

var clients = make(map[*WebSocketClient]bool)

func main() {
	flag.Parse()

	router := gin.Default()

	config := cors.DefaultConfig()
	config.AllowOrigins = []string{"*"} // You can specify specific origins instead of "*"
	config.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	router.Use(cors.New(config))

	router.GET("/ws/payment", func(c *gin.Context) {
		serveWebSocket(c.Writer, c.Request, c)
	})

	router.POST("/createpayment", func(c *gin.Context) {
		body := createPaymentParameter{}

		if err := c.BindJSON(&body); err != nil {
			c.AbortWithError(http.StatusBadRequest, err)
			return
		}
		/////////////////////// Request Access Token ///////////////////////
		tokenUrl := body.TokenApi //e-Payment Access Token API
		tokenReqParams := tokenReq{}
		tokenReqParams.SystemName = body.SystemName
		//jsonStr := []byte(`{"systemName": }`) //DOT Name
		jsonTokenReq, err := json.Marshal(tokenReqParams)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

		req, err := http.NewRequest("POST", tokenUrl, bytes.NewBuffer(jsonTokenReq))
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

		accessToken := accessRes{}

		if err := json.Unmarshal(respBody, &accessToken); err != nil {
			c.AbortWithError(http.StatusBadRequest, err)
			return
		}

		/////////////////////// Request QR payment ///////////////////////
		genQrUrl := body.GenQrApi //e-Payment gen gateway API
		genQrData := genQrParameter{}

		genQrData.SystemName = body.SystemName
		genQrData.Amount = body.Amount
		genQrData.CustomerNo = body.CustomerNo
		genQrData.CustomerUser = body.CustomerUser
		genQrData.Branch = body.Branch
		genQrData.Suffix = body.Suffix
		genQrData.DotTaxId = body.DotTaxId
		genQrData.Description = body.Description
		genQrData.ExpireDate = body.ExpireDate
		genQrData.InvoiceDate = body.InvoiceDate
		genQrData.Reference1 = body.Reference1
		genQrData.Reference2 = body.Reference2

		jsonData, err := json.Marshal(genQrData)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

		req2, err := http.NewRequest("POST", genQrUrl, bytes.NewBuffer(jsonData))
		req2.Header.Set("Content-Type", "application/json")
		req2.Header.Set("x-access-token", accessToken.AccessToken)

		client2 := &http.Client{}
		resp2, err := client2.Do(req2)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

		defer resp2.Body.Close()

		respBody2, err := io.ReadAll(resp2.Body)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

		genQrResp := genQrRes{}

		if err := json.Unmarshal(respBody2, &genQrResp); err != nil {
			c.AbortWithError(http.StatusBadRequest, err)
			return
		}

		c.JSON(http.StatusOK, genQrResp)
	})

	router.POST("/callback", func(c *gin.Context) { //Payment Callback API
		body := CallbackParameter{}
		if err := c.BindJSON(&body); err != nil {
			c.AbortWithError(http.StatusBadRequest, err)
			return
		}

		////////DB update func////////

		/////////////////////// Connect to WebSocket ///////////////////////
		u := url.URL{Scheme: "ws", Host: "localhost:8000", Path: "/ws/payment", RawQuery: "ref=" + body.Reference1}
		//log.Printf("connecting to %s", u.String())

		client, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		if err != nil {
			c.AbortWithError(http.StatusBadRequest, err)
			return
		}
		defer client.Close()

		client.WriteJSON(body) //Send Transaction Result to Client By WebSocket

		c.String(http.StatusOK, "Callback Received") //Http Response
	})

	router.Run(":8000")
}

func serveWebSocket(w http.ResponseWriter, r *http.Request, c *gin.Context) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	client := &WebSocketClient{
		conn:      conn,
		ClientRef: r.URL.Query().Get("ref"),
	}

	clients[client] = true

	for {
		// Read message from Callback API
		_, p, err := conn.ReadMessage()
		if err != nil {
			log.Printf("%s, error while reading message\n", err.Error())
			c.AbortWithError(http.StatusInternalServerError, err)
			break
		}

		wsBody := CallbackParameter{}
		if err := json.Unmarshal(p, &wsBody); err != nil {
			c.AbortWithError(http.StatusBadRequest, err)
			return
		}

		//Send response data to specific clients
		for client := range clients {
			if client.ClientRef == wsBody.Reference2 {
				err := client.conn.WriteJSON(wsBody)
				if err != nil {
					c.AbortWithError(http.StatusBadRequest, err)
					return
				}
			}
		}
	}

	defer func() {
		// When the connection is closed, remove the client from the list
		delete(clients, client)
		client.conn.Close()
	}()
}
