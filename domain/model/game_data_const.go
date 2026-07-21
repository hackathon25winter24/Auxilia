package model

//Default values for game initialization
var DefaultPoints1P = []Position{
	{X: 0, Y: 0},
	{X: 1, Y: 2},
	{X: 0, Y: 4},
}

var DefaultPoints2P = []Position{
	{X: 7, Y: 0},
	{X: 6, Y: 2},
	{X: 7, Y: 4},
}

var DefaultBaseHP = uint(200)
var DefaultCost = uint(50)

var DefaultMap = []Grid{
	{PositionX: 1, PositionY: 1, GridType: 0},
	{PositionX: 2, PositionY: 3, GridType: 0},
	{PositionX: 5, PositionY: 1, GridType: 0},
	{PositionX: 6, PositionY: 3, GridType: 0},
}