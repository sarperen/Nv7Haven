package elements

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Nv7-Github/Nv7Haven/eod/base"
	"github.com/Nv7-Github/Nv7Haven/eod/eodb"
	"github.com/Nv7-Github/Nv7Haven/eod/eodsort"
	"github.com/Nv7-Github/Nv7Haven/eod/trees"
	"github.com/Nv7-Github/Nv7Haven/eod/types"
	"github.com/Nv7-Github/Nv7Haven/eod/util"
	"github.com/bwmarrin/discordgo"
)

const catInfoCount = 3

func (b *Elements) SortCmd(sort string, postfix bool, m types.Msg, rsp types.Rsp) {
	db, res := b.GetDB(m.GuildID)
	if !res.Exists {
		return
	}

	rsp.Acknowledge()

	type sortItem struct {
		id   int
		name string
	}
	items := make([]sortItem, len(db.Elements))

	db.RLock()
	for i, el := range db.Elements {
		items[i] = sortItem{el.ID, el.Name}
	}
	db.RUnlock()

	eodsort.Sort(items, len(items), func(index int) int {
		return items[index].id
	}, func(index int) string {
		return items[index].name
	}, func(index int, val string) {
		items[index].name = val
	}, sort, m.Author.ID, db, postfix)

	text := make([]string, len(items))
	for i, val := range items {
		text[i] = val.name
	}

	b.base.NewPageSwitcher(types.PageSwitcher{
		Kind:       types.PageSwitchInv,
		Title:      db.Config.LangProperty("ElemSort"),
		PageGetter: b.base.InvPageGetter,
		Items:      text,
	}, m, rsp)
}

type catSortInfo struct {
	Name string
	Cnt  int
}

func newCmpCollapsed(db *eodb.DB) discordgo.ActionsRow {
	return discordgo.ActionsRow{
		Components: []discordgo.MessageComponent{
			discordgo.Button{
				Label:    db.Config.LangProperty("InfoExpand"),
				CustomID: "expand",
				Style:    discordgo.SuccessButton,
				Emoji: discordgo.ComponentEmoji{
					Name:     "expand",
					ID:       "932829946706006046",
					Animated: false,
				},
			},
		},
	}
}

func newCmpExpanded(db *eodb.DB) discordgo.ActionsRow {
	return discordgo.ActionsRow{
		Components: []discordgo.MessageComponent{
			discordgo.Button{
				Label:    db.Config.LangProperty("InfoCollapse"),
				CustomID: "collapse",
				Style:    discordgo.DangerButton,
				Emoji: discordgo.ComponentEmoji{
					Name:     "collapse",
					ID:       "932831405640155176",
					Animated: false,
				},
			},
		},
	}
}

type infoComponent struct {
	Expand   *discordgo.MessageEmbed
	Collapse *discordgo.MessageEmbed
	db       *eodb.DB
	Expanded bool

	b *Elements
}

func (c *infoComponent) Handler(_ *discordgo.Session, i *discordgo.InteractionCreate) {
	c.Expanded = !c.Expanded
	var emb *discordgo.MessageEmbed
	var cmp discordgo.ActionsRow
	if c.Expanded {
		emb = c.Expand
		cmp = newCmpExpanded(c.db)
	} else {
		emb = c.Collapse
		cmp = newCmpCollapsed(c.db)
	}
	c.b.dg.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{emb},
			Components: []discordgo.MessageComponent{cmp},
		},
	})
}

func (b *Elements) Info(elem string, id int, isId bool, m types.Msg, rsp types.Rsp) {
	if len(elem) == 0 && !isId {
		return
	}
	db, res := b.GetDB(m.GuildID)
	if !res.Exists {
		rsp.ErrorMessage(res.Message)
		return
	}

	// Get Element name from ID
	var el types.Element
	if isId {
		el, res = db.GetElement(id)
		if !res.Exists {
			rsp.ErrorMessage(res.Message)
			return
		}
	}

	if base.IsFoolsMode && !base.IsFool(elem) {
		rsp.ErrorMessage(base.MakeFoolResp(elem))
		return
	}

	// Get Element
	if !isId {
		el, res = db.GetElementByName(elem)
		if !res.Exists {
			// If what you said was "????", then stop
			if strings.Contains(elem, "?") {
				isValid := false
				for _, letter := range elem {
					if letter != '?' {
						isValid = true
						break
					}
				}
				if !isValid {
					return
				}
			}
			rsp.ErrorMessage(res.Message)
			return
		}
	}
	rsp.Acknowledge()

	// Get Categories
	catsMap := make(map[catSortInfo]types.Empty)
	db.RLock()
	for _, cat := range db.Cats() {
		_, exists := cat.Elements[el.ID]
		if exists {
			catsMap[catSortInfo{
				Name: cat.Name,
				Cnt:  len(cat.Elements),
			}] = types.Empty{}
		}
	}
	db.RUnlock()
	cats := make([]catSortInfo, len(catsMap))
	i := 0
	for k := range catsMap {
		cats[i] = k
		i++
	}

	// Sort by count
	sort.Slice(cats, func(i, j int) bool {
		return cats[i].Cnt > cats[j].Cnt
	})

	// Make text
	catTxt := &strings.Builder{}
	for i := 0; i < catInfoCount && i < len(cats); i++ {
		catTxt.WriteString(cats[i].Name)
		if i != catInfoCount-1 && i != len(cats)-1 {
			catTxt.WriteString(", ")
		}
	}
	if len(cats) > catInfoCount {
		fmt.Fprintf(catTxt, db.Config.LangProperty("InfoAdditionalElemCats"), len(cats)-catInfoCount)
	}

	// Get Madeby
	madeby := 0
	db.RLock()
	for _, comb := range db.Combos() {
		if comb == el.ID {
			madeby++
		}
	}

	// Get foundby
	foundby := 0
	for _, inv := range db.Invs() {
		if inv.Contains(el.ID) {
			foundby++
		}
	}
	db.RUnlock()

	suc, msg, tree := trees.CalcElemInfo(el.ID, m.Author.ID, db)
	if !suc {
		rsp.ErrorMessage(msg)
		return
	}

	if len(el.Comment) == 0 {
		el.Comment = db.Config.LangProperty("DefaultComment")
	}

	shortcomment := el.Comment
	if len(el.Comment) > 100 {
		shortcomment = el.Comment[:99]
	}

	if len(strings.ReplaceAll(shortcomment, "\n", ""))+4 < len(shortcomment) {
		shortcomment = strings.Join(strings.Split(shortcomment, "\n")[:4], "\n")
	}

	if shortcomment != el.Comment {
		shortcomment = strings.TrimSpace(shortcomment) + "..."
	}

	createdOn := fmt.Sprintf("<t:%d>", el.CreatedOn.Unix())
	if el.CreatedOn.Unix() <= 4 {
		createdOn = db.Config.LangProperty("StarterElemCreateTime")
	}

	infoFields := make([]*discordgo.MessageEmbedField, 0)
	if el.Commenter != "" {
		infoFields = append(infoFields, &discordgo.MessageEmbedField{Name: db.Config.LangProperty("InfoCommenter"), Value: fmt.Sprintf("<@%s>", el.Commenter), Inline: true})
	}
	if el.Imager != "" {
		infoFields = append(infoFields, &discordgo.MessageEmbedField{Name: db.Config.LangProperty("InfoImager"), Value: fmt.Sprintf("<@%s>", el.Imager), Inline: true})
	}
	if el.Colorer != "" {
		infoFields = append(infoFields, &discordgo.MessageEmbedField{Name: db.Config.LangProperty("InfoColorer"), Value: fmt.Sprintf("<@%s>", el.Colorer), Inline: true})
	}

	// Make fields
	fullFields := []*discordgo.MessageEmbedField{
		{Name: db.Config.LangProperty("InfoComment"), Value: el.Comment, Inline: false},
		{Name: db.Config.LangProperty("InfoCombosUsedIn"), Value: strconv.Itoa(el.UsedIn), Inline: true},
		{Name: db.Config.LangProperty("InfoCombosMadeWith"), Value: strconv.Itoa(madeby), Inline: true},
		{Name: db.Config.LangProperty("InfoUsersFoundBy"), Value: strconv.Itoa(foundby), Inline: true},
		{Name: db.Config.LangProperty("InfoCreator"), Value: fmt.Sprintf("<@%s>", el.Creator), Inline: true},
		{Name: db.Config.LangProperty("InfoCreateTime"), Value: createdOn, Inline: true},
		{Name: db.Config.LangProperty("InfoColor"), Value: util.FormatHex(el.Color), Inline: true},
		{Name: db.Config.LangProperty("InfoTreeSize"), Value: strconv.Itoa(tree.Total), Inline: true},
		{Name: db.Config.LangProperty("InfoComplexity"), Value: strconv.Itoa(el.Complexity), Inline: true},
		{Name: db.Config.LangProperty("InfoDifficulty"), Value: strconv.Itoa(el.Difficulty), Inline: true},
	}
	fullFields = append(fullFields, infoFields...)

	// Collapsed fields
	fields := []*discordgo.MessageEmbedField{
		{Name: db.Config.LangProperty("InfoComment"), Value: shortcomment, Inline: false},
		{Name: db.Config.LangProperty("InfoCreator"), Value: fmt.Sprintf("<@%s>", el.Creator), Inline: true},
		{Name: db.Config.LangProperty("InfoCreateTime"), Value: createdOn, Inline: true},
		{Name: db.Config.LangProperty("InfoTreeSize"), Value: strconv.Itoa(tree.Total), Inline: true},
	}

	// Get whether has element
	has := db.Config.LangProperty("InfoElemIDUserHasElem")
	inv := db.GetInv(m.Author.ID)
	exists := inv.Contains(el.ID)
	if !exists {
		has = db.Config.LangProperty("InfoElemIDUserNoHasElem")
	}

	// Embed
	emb := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf(db.Config.LangProperty("InfoTitle"), el.Name),
		Description: fmt.Sprintf(has, el.ID, m.Author.ID),
		Fields:      fields,
		Thumbnail: &discordgo.MessageEmbedThumbnail{
			URL: el.Image,
		},
		Color: el.Color,
	}
	if !exists {
		fullFields = append(fullFields, &discordgo.MessageEmbedField{
			Name:   db.Config.LangProperty("InfoElemProgress"),
			Value:  fmt.Sprintf("%s%%", util.FormatFloat(float32(tree.Found)/float32(tree.Total)*100, 2)),
			Inline: true,
		})
	}
	if len(cats) > 0 {
		emb.Fields = append(emb.Fields, &discordgo.MessageEmbedField{Name: db.Config.LangProperty("InfoElemCats"), Value: catTxt.String(), Inline: false})
		fullFields = append(fullFields, &discordgo.MessageEmbedField{Name: db.Config.LangProperty("InfoElemCats"), Value: catTxt.String(), Inline: false})
	}

	if m.Author.ID == "567132457820749842" {
		for _, elem := range base.StarterElements {
			if elem.Name == el.Name {
				emb.Thumbnail.URL = elem.Image
			}
		}
	}

	// Collapsed
	full := *emb
	full.Fields = fullFields

	if len(el.Comment) > 1024 {
		full.Fields = full.Fields[1:]
		full.Description = fmt.Sprintf("%s\n\n**%s**\n%s", emb.Description, db.Config.LangProperty("InfoComment"), el.Comment)
	}

	// Send
	msgId := rsp.RawEmbed(emb, newCmpCollapsed(db))

	// Component
	cmp := &infoComponent{
		b:        b,
		Expand:   &full,
		Collapse: emb,
		db:       db,
	}

	// Data
	data, _ := b.GetData(m.GuildID)
	data.SetMsgElem(msgId, el.ID)
	data.AddComponentMsg(msgId, cmp)
}

func (b *Elements) InfoCmd(elem string, m types.Msg, rsp types.Rsp) {
	db, res := b.GetDB(m.GuildID)
	if !res.Exists {
		rsp.ErrorMessage(res.Message)
		return
	}

	elem = strings.TrimSpace(elem)
	if elem[0] == '#' {
		number, err := strconv.Atoi(elem[1:])
		if err != nil {
			rsp.ErrorMessage(db.Config.LangProperty("InvalidElemID"))
			return
		}
		b.Info("", number, true, m, rsp)
		return
	}
	b.Info(elem, 0, false, m, rsp)
}
