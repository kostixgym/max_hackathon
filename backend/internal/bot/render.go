package bot

import (
	"fmt"
	"html"
	"math/big"
	"strings"
	"time"

	"github.com/max-messenger/max-bot-api-client-go/v2/model"

	"maxhackathon/backend/internal/initiatives"
	"maxhackathon/backend/internal/poll"
	"maxhackathon/backend/internal/registry"
)

// Messages about an initiative go in the HTML format of MAX: the chosen vote and the
// numbers must stand out (проверка 0.6: a plain line with the vote was easy to miss).
// Every text from users or the registry goes through esc: a title with «<a href=…>»
// must not turn into a link in the neighbours' chats.

func esc(s string) string { return html.EscapeString(s) }

// pollView is everything the poll message shows.
type pollView struct {
	initiative initiatives.Polling
	house      registry.HouseRef
	vote       *poll.CastResult // the voter's choice after a press; nil in the invitation
	progress   *poll.Progress   // the support at the moment; nil when unknown
	now        time.Time
	// forInitiator: the initiator does not ask questions, they answer them. Their
	// message tells where the questions come and gives the link for the neighbours.
	forInitiator bool
}

// render builds the poll message: the invitation and, after a press, the same message
// with the voter's choice. After the term the voting buttons are gone; «Есть вопрос»
// stays while the initiative goes on.
func (v pollView) render(me Identity) (string, *model.Keyboard) {
	var b strings.Builder
	fmt.Fprintf(&b, "<b>Опрос соседей: «%s»</b>\n%s\n\n", esc(v.initiative.Title), esc(v.house.Address))
	b.WriteString("Поддерживаете идею? Это опрос мнения соседей, а не голосование собрания: юридической силы у него нет.")

	open := v.initiative.Open(v.now)
	switch {
	case !open:
		b.WriteString("\n\n<b>Опрос завершён.</b>")
	case v.initiative.PollEndsAt != nil:
		fmt.Fprintf(&b, "\n\nОпрос идёт до <b>%s</b>.", formatDeadline(*v.initiative.PollEndsAt, v.house.Location()))
	}
	if v.progress != nil {
		b.WriteString("\n\n" + supportLine(*v.progress))
	}
	if v.vote != nil {
		// <mark> is not visible in the MAX clients (проверка 26.09), bold is.
		fmt.Fprintf(&b, "\n\n<b>Ваш голос: %s</b>", voteSummary(*v.vote))
		if open {
			b.WriteString("\nИзменить голос можно до конца опроса.")
		}
	}
	link := pollLink(me, v.initiative.ID)
	askable := v.initiative.TakesQuestions()
	if v.forInitiator && askable {
		fmt.Fprintf(&b, "\n\n<b>Вы инициатор.</b> Вопросы соседей придут сюда, в этот чат, — ответить можно кнопкой под вопросом. "+
			"Позовите соседей: скопируйте ссылку кнопкой ниже или перешлите <a href=\"%s\">эту ссылку</a> в домовой чат.", esc(link))
	}
	if v.house.IsDemo {
		b.WriteString("\n\n<i>Демо-дом: все данные синтетические.</i>")
	}

	kb := model.NewKeyboard()
	if open {
		kb.AddRow().
			AddButton(voteButton(choiceLabel("Поддерживаю", poll.ChoiceFor, v.vote), v.initiative.ID, "for")).
			AddButton(voteButton(choiceLabel("Против", poll.ChoiceAgainst, v.vote), v.initiative.ID, "against"))
	}
	switch {
	case v.forInitiator && askable:
		kb.AddRow().AddClipboard("Скопировать ссылку для соседей", link)
	case askable:
		kb.AddRow().AddButton(voteButton("Есть вопрос", v.initiative.ID, "question"))
	}
	kb.AddRow().AddButton(appButton(me, "Открыть приложение", v.house.InviteSlug))

	return b.String(), kb
}

// pollStartPrefix marks the start payload of a poll link: max.ru/<bot>?start=poll_<id>.
const pollStartPrefix = "poll_"

// pollLink is the link that brings a neighbour to the poll: the bot sends them the
// poll message (решение 78). The initiator shares it in the chat of the house.
func pollLink(me Identity, initiativeID string) string {
	return "https://max.ru/" + me.Username + "?start=" + pollStartPrefix + initiativeID
}

// choiceLabel marks the button of the current vote.
func choiceLabel(text, choice string, vote *poll.CastResult) string {
	if vote != nil && vote.Choice == choice {
		return "✅ " + text
	}

	return text
}

func voteButton(text, initiativeID, action string) model.Button {
	return model.Button{
		Type:    model.ButtonCallback,
		Text:    text,
		Payload: votePayloadPrefix + ":" + initiativeID + ":" + action,
	}
}

// supportLine is the support at the moment against the threshold of a demand to the
// management company: 10% of the area of the house (ст. 45 ч. 6 ЖК).
func supportLine(p poll.Progress) string {
	if p.DemandReached() {
		return fmt.Sprintf("Поддержали: <b>%s м²</b> — не меньше 10%% площади дома. Этого достаточно, "+
			"чтобы потребовать от УК провести собрание.", registry.FormatM2(p.ForM2()))
	}

	return fmt.Sprintf("Поддержали: <b>%s м²</b> из %s м², нужных, чтобы потребовать от УК провести собрание "+
		"(10%% площади дома).", registry.FormatM2(p.ForM2()), registry.FormatM2(p.Thresholds().Demand))
}

// voteSummary: «за», 26,15 м² (кв. 45).
func voteSummary(result poll.CastResult) string {
	weight := new(big.Rat).SetFrac64(result.WeightNum, result.WeightDen*100) // сотые м² → м²
	word := "«за»"
	if result.Choice == poll.ChoiceAgainst {
		word = "«против»"
	}

	return fmt.Sprintf("%s, %s м² (кв. %s)", word, registry.FormatM2(weight), esc(result.PremiseNumber))
}

// percentOf is part/total in per cent with one decimal: «41,3».
func percentOf(part, total *big.Rat) string {
	if total.Sign() == 0 {
		return "0"
	}
	p := new(big.Rat).Quo(part, total)
	p.Mul(p, big.NewRat(100, 1))

	return strings.Replace(p.FloatString(1), ".", ",", 1)
}

var monthsGenitive = [...]string{
	"января", "февраля", "марта", "апреля", "мая", "июня",
	"июля", "августа", "сентября", "октября", "ноября", "декабря",
}

// formatDeadline shows a moment in the house's local time: «3 октября, 18:00».
func formatDeadline(t time.Time, loc *time.Location) string {
	local := t.In(loc)

	return fmt.Sprintf("%d %s, %02d:%02d", local.Day(), monthsGenitive[local.Month()-1], local.Hour(), local.Minute())
}
