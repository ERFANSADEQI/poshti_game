package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"poshti/poshti"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	qrCode "github.com/skip2/go-qrcode"
	"golang.org/x/term"
)

type Model struct {
	choices            []string
	cursor             int
	selected           map[int]struct{}
	screen             int
	textInput          textinput.Model
	playerName         string
	friendName         string
	message            string
	coins              [10]int
	player1Sum         int
	player2Sum         int
	turn               int
	waitingForReply    bool
	invitationRejected bool
	startFromLeft      bool
	coinCursor         int
	selectedCoins      [10]bool
	firstActive        int
	lastActive         int
	playWithComputer   bool
	client             *poshti.Client
	qrCode             string
	requestMessage     string
	pendingRequests    []string
	requestResult      string
	friendInvitation   string
}

const (
	QrcodeScreen  = 0
	helpScreen    = 1
	menuScreen    = 2
	friendScreen  = 3
	gameScreen    = 4
	resultScreen  = 5
	requestScreen = 6
)

func InitialModel() Model {
	terminalWidth, _, err := term.GetSize(0)
	if err != nil {
		terminalWidth = 80
	}
	ti := textinput.New()
	ti.Placeholder = "example: mamad"
	ti.Focus()
	ti.CharLimit = 10
	ti.Width = terminalWidth / 2

	client := poshti.NewClient("04f875b8-ce46-4800-8d7c-295d9779efb9")
	err = client.Connect("eyJhbGciOiJIUzI1NiJ9.eyJjYyI6IjIwIiwibXBkIjoiNTAwIiwibmFtZSI6InJlcGxpY2EiLCJwX3VpZCI6IjA0Zjg3NWI4LWNlNDYtNDgwMC04ZDdjLTI5NWQ5Nzc5ZWZiOSJ9.ST_S7vC3lvSO-D1JouFv8D37IkbJCOalQwcgVlWDFCQ")
	if err != nil {
		log.Fatal("Could not connect to poshti server: ", err)
	}

	m := Model{
		choices:          []string{"Play with my friend", "Play with computer"},
		selected:         make(map[int]struct{}),
		turn:             1,
		textInput:        ti,
		screen:           0,
		coinCursor:       0,
		playWithComputer: false,
		client:           client,
		pendingRequests:  []string{},
		requestResult:    "",
	}

	client.JoinChannel("test", func(channel, topic string, payload interface{}) {
		fmt.Printf("Received message on channel %s, topic %s: %s\n", channel, topic, payload)
		if message, ok := payload.(string); ok {
			parts := strings.SplitN(message, ":", 2)
			if len(parts) == 2 {
				command, value := parts[0], parts[1]
				switch command {
				case "friend_request":
					m.pendingRequests = append(m.pendingRequests, value)
					log.Printf("Friend request received from: %s", value)
					m.screen = requestScreen
					log.Printf("Current pending requests: %v", m.pendingRequests)
				case "friend_accept":
					log.Printf("Friend request accepted by %s!", value)
					m.screen = gameScreen
					m.generateCoins(m.playWithComputer)
				case "friend_decline":
					log.Println("Friend request declined.")
					m.requestResult = "Your friend declined the request :("
					m.pendingRequests = nil
				}
			}
		}
	})

	m.qrCode = generateQRCode("https://app.poshti.live/start")
	m.updateActiveCoins()
	return m
}

func (m *Model) acceptRequest(friend string) {
	responseMessage := fmt.Sprintf("friend_accept:%s", m.playerName)
	if err := m.client.Send("test", "friend_accept", responseMessage); err != nil {
		log.Println("Error accepting friend request:", err)
	} else {
		m.screen = gameScreen
		m.generateCoins(m.playWithComputer)
	}
}

func (m *Model) declineRequest(friend string) {
	responseMessage := fmt.Sprintf("friend_decline:%s", friend)
	if err := m.client.Send("test", "friend_decline", responseMessage); err != nil {
		log.Println("Error declining friend request:", err)
	} else {
		m.requestResult = "You declined the request."
		m.pendingRequests = nil
		m.screen = menuScreen
	}
}

func generateQRCode(url string) string {
	var qrCodeArt string
	png, err := qrCode.New(url, qrCode.Low)
	if err != nil {
		log.Fatal(err)
	}
	qrCodeArt = png.ToSmallString(false)
	return qrCodeArt
}

func (m *Model) generateCoins(playWithComputer bool) {
	rand.Seed(time.Now().UnixNano())

	if !playWithComputer {
		for i := 0; i < len(m.coins); i++ {
			m.coins[i] = rand.Intn(50) + 1
		}
	} else {
		a := rand.Intn(30) + 1
		b := rand.Intn(30) + 1

		if a > b {
			m.coins[0] = a
			m.coins[len(m.coins)-1] = b
		} else {
			m.coins[0] = b
			m.coins[len(m.coins)-1] = a
		}

		for i := 1; i < len(m.coins)-1; i += 2 {
			m.coins[i] = rand.Intn(50-m.coins[i-1]) + m.coins[i-1] + 1

			randomNum := rand.Intn(50) + 1

			if randomNum > m.coins[len(m.coins)-1] {
				m.coins[i+1] = randomNum
			} else {
				m.coins[i+1] = m.coins[len(m.coins)-1]
				m.coins[len(m.coins)-1] = randomNum
			}
		}
	}
}

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "enter":
			if m.screen == QrcodeScreen {
				m.screen = helpScreen
			} else if m.screen == friendScreen {
				if len(m.pendingRequests) > 0 {
					m.screen = requestScreen
				} else {
					inputValue := m.textInput.Value()
					if inputValue != "" {
						m.handleRequest(inputValue)
						m.textInput.Reset()
					}
				}
			} else if m.screen == requestScreen {
				if len(m.pendingRequests) > 0 {
					if m.cursor == 0 {
						m.acceptRequest(m.pendingRequests[0])
					} else if m.cursor == 1 {
						m.declineRequest(m.pendingRequests[0])
					}
					m.pendingRequests = m.pendingRequests[1:]
					if len(m.pendingRequests) == 0 {
						m.screen = friendScreen
					}
				}
			} else if m.screen == gameScreen {
				m.handleCoinSelection()
				if m.playWithComputer && m.turn == 2 {
					m.handleComputerSelection()
				}
			} else if m.screen == resultScreen {
				m.screen = QrcodeScreen
				m.resetGame()
			} else if m.screen == helpScreen {
				m.screen = menuScreen
			} else {
				return m.handleEnter()
			}
		case "left":
			if m.screen == gameScreen {
				m.MoveCursorLeft()
			} else if m.cursor > 0 {
				m.cursor--
			}
		case "right":
			if m.screen == gameScreen {
				m.MoveCursorRight()
			} else if m.cursor < len(m.choices)-1 {
				m.cursor++
			}
		case "backspace":
			if m.screen == menuScreen && len(m.playerName) > 0 {
				m.playerName = m.playerName[:len(m.playerName)-1]
			} else if m.screen == friendScreen && len(m.friendName) > 0 {
				m.friendName = m.friendName[:len(m.friendName)-1]
			}
		default:
			if m.screen == menuScreen && len(m.playerName) < 20 {
				m.playerName += msg.String()
			} else if m.screen == friendScreen && len(m.friendName) < 20 {
				m.friendName += msg.String()
			}
		}
		m.textInput, cmd = m.textInput.Update(msg)
	}
	return m, cmd
}

func (m *Model) handleRequest(friend string) {
	requestMessage := fmt.Sprintf("friend_request:%s", m.playerName)
	if err := m.client.Send("test", "friend_request", requestMessage); err != nil {
		log.Println("Error sending friend request:", err)
	} else {
		m.message = fmt.Sprintf("Waiting for %s to accept your request...", friend)
	}
}

func (m *Model) handleResponse(accepted bool) {
	if accepted {
		m.screen = gameScreen
		m.generateCoins(m.playWithComputer)
	} else {
		m.requestResult = "Your friend declined the request :("
		m.pendingRequests = nil
	}
}

func (m *Model) updateRequestMessage(condition string) {
	switch condition {
	case "noRequests":
		m.requestMessage = "You have no requests for a game :("
	case "noFriendWithName":
		m.requestMessage = "No friend with this name exists :("
	default:
		m.requestMessage = ""
	}
}

func (m *Model) resetGame() {
	m.selectedCoins = [10]bool{}
	m.playerName = ""
	m.friendName = ""
	m.player1Sum = 0
	m.player2Sum = 0
	m.turn = 1
	m.coinCursor = 0
	m.textInput.Reset()
	m.textInput.Focus()
	m.updateActiveCoins()
}

func resetTextInput(ti *textinput.Model) {
	ti.Reset()
	ti.Placeholder = "example: mamad"
	ti.Focus()
}

func (m *Model) friendExists(name string) bool {
	return true
}

func (m *Model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.screen {
	case menuScreen:
		inputValue := m.textInput.Value()
		if inputValue != "" {
			m.playerName = inputValue
			if m.cursor == 0 {
				m.screen = friendScreen
				m.updateRequestMessage("noRequests")
				resetTextInput(&m.textInput)
				m.generateCoins(false)
			} else if m.cursor == 1 {
				m.friendName = "Computer"
				m.screen = gameScreen
				m.playWithComputer = true
				m.generateCoins(true)
			}
			m.message = ""
		}
	case friendScreen:
		inputValue := m.textInput.Value()
		if inputValue != "" {
			m.friendName = inputValue
			if m.friendExists(inputValue) {
				m.screen = gameScreen
				resetTextInput(&m.textInput)
			} else {
				m.updateRequestMessage("noFriendWithName")
			}
		}
	}
	return m, nil
}

func (m *Model) handleCoinSelection() {
	if m.coins[m.coinCursor] != 0 {
		m.updateGame(m.coinCursor)
	}
}

func (m *Model) handleComputerSelection() {
	var availableCoins []int
	for i, selected := range m.selectedCoins {
		if !selected && i != m.coinCursor {
			availableCoins = append(availableCoins, i)
		}
	}

	if len(availableCoins) >= 2 {
		firstCoinIndex := availableCoins[0]
		lastCoinIndex := availableCoins[len(availableCoins)-1]
		var coinIndex int
		if m.coins[firstCoinIndex] >= m.coins[lastCoinIndex] {
			coinIndex = firstCoinIndex
		} else {
			coinIndex = lastCoinIndex
		}
		m.updateGame(coinIndex)
	} else if len(availableCoins) == 1 {
		coinIndex := availableCoins[0]
		m.updateGame(coinIndex)
	}
}

func (m *Model) updateGame(coinIndex int) {
	if m.turn == 1 {
		m.player1Sum += m.coins[coinIndex]
		m.turn = 2
		if m.playWithComputer {
			m.handleComputerSelection()
		}
	} else {
		m.player2Sum += m.coins[coinIndex]
		m.turn = 1
	}
	m.selectedCoins[coinIndex] = true
	m.updateActiveCoins()

	if m.allCoinsPicked() {
		m.screen = resultScreen
	}
}

func (m *Model) updateActiveCoins() {
	m.firstActive, m.lastActive = -1, -1
	for i, selected := range m.selectedCoins {
		if !selected {
			if m.firstActive == -1 {
				m.firstActive = i
			}
			m.lastActive = i
		}
	}
	if m.coinCursor != m.firstActive && m.coinCursor != m.lastActive {
		if m.startFromLeft {
			m.coinCursor = m.firstActive
		} else {
			m.coinCursor = m.lastActive
		}
	}
}

func (m *Model) MoveCursorLeft() {
	if m.coinCursor == m.firstActive {
		m.coinCursor = m.lastActive
	} else {
		m.coinCursor = m.firstActive
	}
}

func (m *Model) MoveCursorRight() {
	if m.coinCursor == m.lastActive {
		m.coinCursor = m.firstActive
	} else {
		m.coinCursor = m.lastActive
	}
}

func (m Model) View() string {
	terminalWidth, _, err := term.GetSize(0)
	if err != nil {
		terminalWidth = 80
	}

	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#000000")).Background(lipgloss.Color("#FFD700")).Padding(1, 2, 1, 2).Bold(true).Italic(true).Width(terminalWidth).Align(lipgloss.Center)
	style2 := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF4500")).Padding(1, 2, 1, 2).Bold(true).Italic(true).Width(terminalWidth).Align(lipgloss.Center)

	text1 := "Collect Coins"
	text2 := "Enter Your Name: "
	styledText1 := style.Render(text1)
	styledText2 := style2.Render(text2)

	styledTextInput := lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("63")).Padding(1).Render(m.textInput.View())

	centeredText1 := lipgloss.Place(terminalWidth, 5, lipgloss.Center, lipgloss.Center, styledText1)
	centeredText2 := lipgloss.Place(terminalWidth, 5, lipgloss.Center, lipgloss.Center, styledText2)
	centeredInput := lipgloss.Place(terminalWidth, 3, lipgloss.Center, lipgloss.Center, styledTextInput)

	var s string
	switch m.screen {
	case helpScreen:
		s += m.viewHelpScreen()
	case menuScreen:
		s += fmt.Sprintf("%s\n\n%s\n%s\n%s", centeredText1, centeredText2, centeredInput, m.viewMenu())
	case friendScreen:
		friendPrompt := fmt.Sprintf("Hello %s! What's your friend's name?", m.playerName)
		friendPromptStyled := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF4500")).Render(friendPrompt)
		centeredFriendPrompt := lipgloss.Place(terminalWidth, 3, lipgloss.Center, lipgloss.Center, friendPromptStyled)

		inputBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("63")).
			Padding(1, 2).
			Render(m.textInput.View())
		centeredInputBox := lipgloss.Place(terminalWidth, 5, lipgloss.Center, lipgloss.Center, inputBox)

		requestTitle := "Requests"
		requestTitleStyled := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00FF00")).
			Bold(true).
			Render(requestTitle)
		centeredRequestTitle := lipgloss.Place(terminalWidth, 1, lipgloss.Center, lipgloss.Center, requestTitleStyled)

		requestMessage := fmt.Sprintf("%s", m.requestMessage)
		requestMessageStyled := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF5733")).
			Render(requestMessage)
		centeredRequestMessage := lipgloss.Place(terminalWidth, 1, lipgloss.Center, lipgloss.Center, requestMessageStyled)

		halfTerminalWidth := terminalWidth / 2
		line := strings.Repeat("-", halfTerminalWidth)
		centeredLine := fmt.Sprintf("%*s", (terminalWidth+len(line))/2, line)

		centeredMessage := lipgloss.Place(terminalWidth, 3, lipgloss.Center, lipgloss.Center, m.message)

		var requestsView string
		for i, req := range m.pendingRequests {
			cursor := " "
			if m.cursor == i {
				cursor = ">"
			}
			acceptButton := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#00FF00")).
				Bold(m.cursor == i && m.cursor == 0).
				Render("Accept")
			declineButton := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FF0000")).
				Bold(m.cursor == i && m.cursor == 1).
				Render("Decline")
			requestMsg := fmt.Sprintf("%s %s wants to play with you %s  %s", cursor, req, acceptButton, declineButton)
			requestsView += requestMsg + "\n"
		}
		requestsViewStyled := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF4500")).Render(requestsView)
		centeredRequestsView := lipgloss.Place(terminalWidth, 3, lipgloss.Center, lipgloss.Center, requestsViewStyled)

		s += fmt.Sprintf("%s\n%s\n\n%s\n%s\n%s\n%s\n%s", centeredFriendPrompt, centeredInputBox, centeredRequestTitle, centeredLine, centeredRequestsView, centeredRequestMessage, centeredMessage)

	case gameScreen:
		s += m.viewGameScreen()
	case resultScreen:
		s += m.viewResultScreen()
	case QrcodeScreen:
		s += m.viewQrcodeScreen()
	case requestScreen:
		s += m.viewRequestScreen()
	default:
		s += "Unknown screen!"
	}
	quitText := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Render("Press 'Ctrl + C' to quit.")
	s += lipgloss.Place(terminalWidth, 4, lipgloss.Left, lipgloss.Bottom, quitText)
	return s
}

func (m Model) viewMenu() string {
	optionStyle := lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#000000")).Padding(1, 3).Background(lipgloss.Color("#FFD700")).Foreground(lipgloss.Color("#000000"))
	selectedStyle := optionStyle.Copy().BorderForeground(lipgloss.Color("#FF4500")).Background(lipgloss.Color("#FF4500")).Foreground(lipgloss.Color("#FFFFFF"))

	var choices []string
	for i, choice := range m.choices {
		var renderedChoice string
		if m.cursor == i {
			renderedChoice = selectedStyle.Render(choice)
		} else {
			renderedChoice = optionStyle.Render(choice)
		}
		choices = append(choices, renderedChoice)
	}

	terminalWidth, _, err := term.GetSize(0)
	if err != nil {
		terminalWidth = 80
	}
	menu := lipgloss.JoinHorizontal(lipgloss.Center, choices...)
	return lipgloss.PlaceHorizontal(terminalWidth, lipgloss.Center, menu)
}

func (m Model) viewGameScreen() string {
	terminalWidth, _, err := term.GetSize(0)
	if err != nil {
		terminalWidth = 80
	}

	coinStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#000")).Background(lipgloss.Color("#FFD700")).Padding(1, 3).BorderStyle(lipgloss.RoundedBorder()).Align(lipgloss.Center)
	selectedCoinStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFF")).Background(lipgloss.Color("#FF4500")).Padding(1, 3).BorderStyle(lipgloss.RoundedBorder()).Align(lipgloss.Center).Bold(true)
	highlightedCoinStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFF")).Background(lipgloss.Color("#32CD32")).Padding(1, 3).BorderStyle(lipgloss.RoundedBorder()).Align(lipgloss.Center).Bold(true)
	selectedOldCoinStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFF")).Background(lipgloss.Color("#0000FF")).Padding(1, 3).BorderStyle(lipgloss.RoundedBorder()).Align(lipgloss.Center).Bold(true)

	var coins []string
	for i, coin := range m.coins {
		var styledCoin string
		if m.selectedCoins[i] {
			styledCoin = selectedOldCoinStyle.Render(fmt.Sprintf("%d", coin))
		} else if i == m.coinCursor {
			styledCoin = selectedCoinStyle.Render(fmt.Sprintf("%d", coin))
		} else if i == m.firstActive || i == m.lastActive {
			styledCoin = highlightedCoinStyle.Render(fmt.Sprintf("%d", coin))
		} else {
			styledCoin = coinStyle.Render(fmt.Sprintf("%d", coin))
		}
		coins = append(coins, styledCoin)
	}

	coinsDisplay := lipgloss.JoinHorizontal(lipgloss.Center, coins...)
	centeredCoins := lipgloss.PlaceHorizontal(terminalWidth, lipgloss.Center, coinsDisplay)

	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD700")).Background(lipgloss.Color("#333")).Bold(true).Align(lipgloss.Center).Padding(0, 1).BorderStyle(lipgloss.RoundedBorder()).Margin(0, 1)

	nameActiveStyle := nameStyle.Copy().Background(lipgloss.Color("#FFD700")).Foreground(lipgloss.Color("#000000")).Bold(true)

	var player1NameStyle, player2NameStyle lipgloss.Style
	player1NameStyle = nameStyle
	player2NameStyle = nameStyle

	if m.turn == 1 {
		player1NameStyle = nameActiveStyle
	} else {
		player2NameStyle = nameActiveStyle
	}

	scoreStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFF")).Background(lipgloss.Color("#333")).Bold(true).Align(lipgloss.Center).Padding(0, 1).BorderStyle(lipgloss.RoundedBorder()).Margin(0, 1)

	opponentName := m.friendName
	if opponentName == "" {
		opponentName = "Computer"
	}

	player1Info := lipgloss.JoinVertical(lipgloss.Center,
		player1NameStyle.Render(fmt.Sprintf("%s", m.playerName)),
		scoreStyle.Render(fmt.Sprintf("%d", m.player1Sum)),
	)

	player2Info := lipgloss.JoinVertical(lipgloss.Center,
		player2NameStyle.Render(fmt.Sprintf("%s", opponentName)),
		scoreStyle.Render(fmt.Sprintf("%d", m.player2Sum)),
	)

	spacer := lipgloss.NewStyle().Width(5).Render(" ")
	playerInfo := lipgloss.JoinHorizontal(lipgloss.Center, player1Info, spacer, player2Info)
	centeredPlayerInfo := lipgloss.PlaceHorizontal(terminalWidth, lipgloss.Center, playerInfo)

	centeredCoinsWithHeader := lipgloss.JoinVertical(lipgloss.Center,
		lipgloss.NewStyle().Foreground(lipgloss.Color("#000")).Background(lipgloss.Color("#FFD700")).Padding(1, 2).Bold(true).Align(lipgloss.Center).Width(terminalWidth).MarginBottom(3).Render("Coins"),
		centeredCoins,
	)

	s := fmt.Sprintf("%s\n\n%s\n\nChoose a coin with left/right keys and press Enter to select\n\n",
		centeredCoinsWithHeader,
		centeredPlayerInfo,
	)
	return s
}

func (m Model) allCoinsPicked() bool {
	for _, selected := range m.selectedCoins {
		if !selected {
			return false
		}
	}
	return true
}

func (m Model) viewRequestScreen() string {
	terminalWidth, _, err := term.GetSize(0)
	if err != nil {
		terminalWidth = 80
	}

	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#000000")).Background(lipgloss.Color("#FFD700")).Padding(1, 2, 1, 2).Bold(true).Italic(true).Width(terminalWidth).Align(lipgloss.Center)
	requestTitle := "Game Requests"
	styledTitle := style.Render(requestTitle)

	var requestsView string
	for i, req := range m.pendingRequests {
		cursor := " "
		if m.cursor == i {
			cursor = ">"
		}
		acceptButton := lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Render("Accept")
		declineButton := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Render("Decline")
		requestMsg := fmt.Sprintf("%s %s [%s] [%s]", cursor, req, acceptButton, declineButton)
		requestsView += requestMsg + "\n"
	}
	requestsViewStyled := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF4500")).Render(requestsView)
	centeredRequestsView := lipgloss.Place(terminalWidth, 5, lipgloss.Center, lipgloss.Center, requestsViewStyled)
	return fmt.Sprintf("%s\n\n%s", styledTitle, centeredRequestsView)
}

func (m Model) viewQrcodeScreen() string {
	terminalWidth, terminalHeight, err := term.GetSize(0)
	if err != nil {
		terminalWidth = 80
		terminalHeight = 24
	}

	helpText := `
Welcome to our game! Here's how to get started:
1. Use the arrow keys to move your character.
2. Avoid obstacles and collect points.
3. Reach the end of each level to progress.

Enjoy the game and have fun!

Press Enter to continue...
`
	qrCodeStyle := lipgloss.NewStyle().
		Align(lipgloss.Center).
		Render(m.qrCode)

	styledHelpText := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color("#333333")).
		Padding(0, 1).
		Border(lipgloss.RoundedBorder()).
		Render(helpText)

	contentHeight := terminalHeight - 2

	content := lipgloss.JoinVertical(lipgloss.Center, styledHelpText, qrCodeStyle)

	return lipgloss.Place(terminalWidth, contentHeight, lipgloss.Center, lipgloss.Center, content)
}

func (m Model) viewHelpScreen() string {
	terminalWidth, _, err := term.GetSize(0)
	if err != nil {
		terminalWidth = 80
	}

	helpText := `
How to Play:
	
1. Use the left and right arrow keys to navigate through the coins.
2. Press Enter to select a coin.
3. Players take turns selecting coins. The goal is to collect coins with the highest values.
4. The game ends when all coins are collected. The player with the highest total value wins.
	
Coin Colors:
- Green: Active coin (available for selection)
- Blue: Inactive coin (already selected)
- Orange: Selected coin (current selection)
`

	activeCoinStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#32CD32")).Render("Green")
	inactiveCoinStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#0000FF")).Render("Blue")
	selectedCoinStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF4500")).Render("Orange")

	helpText = fmt.Sprintf(`
How to Play:
	
1. Use the left and right arrow keys to navigate through the coins.
2. Press Enter to select a coin.
3. Players take turns selecting coins. The goal is to collect coins with the highest values.
4. The game ends when all coins are collected. The player with the highest total value wins.
	
Coin Colors:
- %s: Active coin (available for selection)
- %s: Inactive coin (already selected)
- %s: Selected coin (current selection)
`, activeCoinStyle, inactiveCoinStyle, selectedCoinStyle)

	styledHelpText := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color("#333333")).
		Padding(1, 2).
		Border(lipgloss.RoundedBorder()).
		Render(helpText)

	Start := lipgloss.NewStyle().
		Align(lipgloss.Center).
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color("#FF4500")).
		Padding(1, 2).
		Border(lipgloss.RoundedBorder()).
		Render("Start Game")
	content := lipgloss.JoinVertical(lipgloss.Center, styledHelpText, Start)
	return lipgloss.Place(terminalWidth, 20, lipgloss.Center, lipgloss.Center, content)
}

func (m Model) viewResultScreen() string {
	var message string

	if m.player1Sum > m.player2Sum {
		message = "Congratulations, you win :)"
	} else {
		message = "Unfortunately you lost :("
	}

	terminalWidth, terminalHeight, err := term.GetSize(0)
	if err != nil {
		terminalWidth = 80
		terminalHeight = 24
	}

	innerBox := lipgloss.NewStyle().
		Align(lipgloss.Center).
		Foreground(lipgloss.Color("#FFD700")).
		Background(lipgloss.Color("#4B0082")).
		Padding(1, 4).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#9400D3")).
		Render(message)
	outerBox := lipgloss.NewStyle().
		Align(lipgloss.Center).
		Padding(1, 2).
		Border(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color("#FFD700")).
		Render(innerBox)
	playAgain := lipgloss.NewStyle().
		Align(lipgloss.Center).
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color("#FF4500")).
		Padding(1, 2).
		Border(lipgloss.RoundedBorder()).
		Render("Play Again")

	content := lipgloss.JoinVertical(lipgloss.Center, outerBox, playAgain)

	return lipgloss.Place(terminalWidth, terminalHeight, lipgloss.Center, lipgloss.Center, content)

}

func main() {
	p := tea.NewProgram(InitialModel(), tea.WithAltScreen())
	if err := p.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}
