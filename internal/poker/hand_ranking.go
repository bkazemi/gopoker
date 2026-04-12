package poker

import (
	"fmt"

	"github.com/rs/zerolog/log"

	"github.com/rivo/uniseg"
)

func checkTies(players []*Player, cardidx int) []*Player {
	if len(players) == 1 || cardidx == -1 {
		// one player left or remaining players tied fully
		return players
	}

	best := []*Player{players[0]}

	for _, player := range players[1:] {
		if player.Hand.Cards[cardidx].NumValue == best[0].Hand.Cards[cardidx].NumValue {
			best = append(best, player)
		} else if player.Hand.Cards[cardidx].NumValue > best[0].Hand.Cards[cardidx].NumValue {
			best = []*Player{player}
		}
	}

	return checkTies(best, cardidx-1)
}

func (table *Table) BestHand(players []*Player, sidePot *SidePot) []*Player {
	var winInfo *string
	if sidePot == nil {
		winInfo = &table.WinInfo
		//table.WinInfo = table.CommunityToString() + "\n\n"
		*winInfo = table.CommunityToString() + "\n\n"
	} else {
		winInfo = &sidePot.WinInfo
	}

	// XXX move me
	maxNameWidth := 0
	for _, player := range players {
		maxNameWidth = max(uniseg.StringWidth(player.Name), maxNameWidth)
	}

	if sidePot == nil {
		for _, player := range players {
			AssembleBestHand(false, table, player)

			nameField := FillRight(player.Name, maxNameWidth)

			*winInfo += fmt.Sprintf("%s [%4s][%4s] => %-15s (rank %d)\n",
				nameField,
				player.Hole.Cards[0].Name, player.Hole.Cards[1].Name,
				player.Hand.RankName(), player.Hand.Rank)

			log.Debug().Str("player", nameField).
				Str("hole", fmt.Sprintf("[%4s][%4s]", player.Hole.Cards[0].Name, player.Hole.Cards[1].Name)).
				Str("hand", player.Hand.RankName()).Int("rank", int(player.Hand.Rank)).
				Msg("player hand")
		}
	}

	bestPlayers := []*Player{players[0]}

	for _, player := range players[1:] {
		if player.Hand.Rank == bestPlayers[0].Hand.Rank {
			bestPlayers = append(bestPlayers, player)
		} else if player.Hand.Rank > bestPlayers[0].Hand.Rank {
			bestPlayers = []*Player{player}
		}
	}

	tiedPlayers := checkTies(bestPlayers, 4)

	if len(tiedPlayers) > 1 {
		// split pot
		names := ""
		*winInfo += "split pot between "
		for _, player := range tiedPlayers {
			names += player.Name + " "
			*winInfo += player.Name + " "
		}
		log.Info().
			Str("players", names).
			Str("hand", tiedPlayers[0].Hand.RankName()).
			Msg("split pot")

		*winInfo += "\nwinning hand => " + tiedPlayers[0].Hand.RankName() + "\n"
	} else {
		*winInfo += "\n" + tiedPlayers[0].Name + "  wins with " + tiedPlayers[0].Hand.RankName() + "\n"
		log.Info().
			Str("player", tiedPlayers[0].Name).
			Str("hand", tiedPlayers[0].Hand.RankName()).
			Msg("winner")
	}

	// build best hand string
	handStr := ""
	for _, card := range reverseCards(tiedPlayers[0].Hand.Cards) {
		handStr += fmt.Sprintf("[%4s]", card.Name)
		*winInfo += fmt.Sprintf("[%4s]", card.Name)
	}
	log.Debug().Str("cards", handStr).Msg("best hand")

	return tiedPlayers
}

// hand matching logic unoptimized
func AssembleBestHand(preShow bool, table *Table, player *Player) {
	if preShow {
		cloneHand := func(hand *Hand) Hand {
			if hand == nil {
				return Hand{}
			}

			cloned := *hand
			cloned.Cards = append(Cards(nil), hand.Cards...)

			return cloned
		}

		var restoreHand Hand
		if player.Hand != nil {
			restoreHand = cloneHand(player.Hand)
		} else {
			restoreHand = Hand{}
		}

		defer func() {
			if preShow {
				if table.State == TableStatePreFlop && len(player.Hole.Cards) == 2 {
					player.preHand = &Hand{}
					if player.Hole.Cards[0].NumValue == player.Hole.Cards[1].NumValue {
						player.preHand.Rank = RankPair
					}
				} else if player.Hand != nil {
					previewHand := cloneHand(player.Hand)
					player.preHand = &previewHand
				} else {
					player.preHand = &Hand{}
				}
				player.Hand = &restoreHand
			}
		}()
	}

	if table.State == TableStatePreFlop ||
		len(player.Hole.Cards) != 2 ||
		len(table.Community) < 3 {
		return
	}

	cards := append(Cards{}, table.Community...)
	cards = append(cards, player.Hole.Cards...)
	cardsSort(&cards)
	bestCard := len(cards)

	// get all the pairs/trips/quads into one slice
	// NOTE: ascending order
	//
	// this struct keeps a slice of the match type indexes
	// in ascending order
	var matchHands struct {
		quads []uint
		trips []uint
		pairs []uint
	}

	matching_cards := false

	// NOTE: double loop not optimal, readability trade-off okay for given slice size
	for i := 0; i < len(cards)-1; {
		match_num := 1

		for _, adj_card := range cards[i+1:] {
			if cards[i].NumValue == adj_card.NumValue {
				match_num++
			} else {
				break
			}
		}

		if match_num > 1 {
			if !matching_cards {
				matching_cards = true
			}

			var matchmemb *[]uint
			switch match_num {
			case 4:
				matchmemb = &matchHands.quads
			case 3:
				matchmemb = &matchHands.trips
			case 2:
				matchmemb = &matchHands.pairs
			}
			*matchmemb = append(*matchmemb, uint(i))

			i += match_num
		} else {
			i++
		}
	}

	// used for tie breakers
	// this func assumes the card slice is sorted and ret will be <= 5
	top_cards := func(cards Cards, num int, except []CardVal) Cards {
		ret := make(Cards, 0, 5)

		Assert(len(cards) <= 7, "too many cards in top_cards()")

		for i := len(cards) - 1; i >= 0; i-- {
			skip := false
			if len(ret) == num {
				return ret
			}

			for _, except_numvalue := range except {
				if cards[i].NumValue == except_numvalue {
					skip = true
					break
				}
			}

			if !skip {
				// insert at beginning of slice
				ret = append(Cards{cards[i]}, ret...)
			}
		}

		return ret
	}

	// flush search function //
	gotFlush := func(cards Cards, player *Player, addToCards bool) (bool, Suit) {
		type _suitstruct struct {
			cnt   uint
			cards Cards
		}
		suits := make(map[Suit]*_suitstruct)

		for _, card := range cards {
			suits[card.Suit] = &_suitstruct{cards: Cards{}}
		}

		// count each suit
		for _, card := range cards {
			suits[card.Suit].cnt++
			suits[card.Suit].cards = append(suits[card.Suit].cards, card)
		}

		// search for flush
		for suit, suit_struct := range suits {
			if suit_struct.cnt >= 5 { // NOTE: it's only possible to get one flush
				player.Hand.Rank = RankFlush

				if addToCards {
					player.Hand.Cards = append(player.Hand.Cards,
						suit_struct.cards[len(suit_struct.cards)-5:len(suit_struct.cards)]...)
				}
				return true, suit
			}
		}

		return false, 0
	}

	// straight flush/straight search function //
	gotStraight := func(cards *Cards, player *Player, high int, acelow bool) bool {
		straightFlush := true

		if acelow {
			// check ace to 5
			acesuit := (*cards)[len(*cards)-1].Suit

			if (*cards)[0].NumValue != CardTwo {
				return false // can't be A to 5
			}

			for i := 1; i <= high; i++ {
				if (*cards)[i].Suit != acesuit {
					straightFlush = false
				}

				if (*cards)[i].NumValue != (*cards)[i-1].NumValue+1 {
					return false
				}
			}
		} else {
			low := high - 4
			for i := high; i > low; i-- {
				//fmt.Printf("h %d L %d ci %d ci-1 %d\n", high, low, i, i-1)
				if (*cards)[i].Suit != (*cards)[i-1].Suit { // XXX had [i-1].Suit+1 for some reason
					straightFlush = false
				}

				if (*cards)[i].NumValue != (*cards)[i-1].NumValue+1 {
					return false
				}
			}
		}

		if straightFlush {
			if (*cards)[high].NumValue == CardAce {
				player.Hand.Rank = RankRoyalFlush
			} else {
				player.Hand.Rank = RankStraightFlush
			}
		} else {
			player.Hand.Rank = RankStraight
		}

		if acelow {
			player.Hand.Cards = append(Cards{(*cards)[len(*cards)-1]}, (*cards)[:4]...)
		} else {
			player.Hand.Cards = append(player.Hand.Cards, (*cards)[high-4:high+1]...)
		}
		Assert(len(player.Hand.Cards) == 5, fmt.Sprintf("%d", len(player.Hand.Cards)))

		return true
	}

	if !matching_cards {
		// best possible hands with no matches in order:
		// royal flush, straight flush, flush, straight or high card.
		// XXX: make better
		// we check for best straight first to reduce cycles
		//for i := 1; i < 4; i++ {
		isStraight := false

		for i := 1; i < len(cards)-3; i++ {
			if gotStraight(&cards, player, bestCard-i, false) {
				isStraight = true
				break
			}
		}

		if player.Hand.Rank == RankRoyalFlush ||
			player.Hand.Rank == RankStraightFlush {
			return
		}

		// check for A to 5
		if !isStraight && cards[len(cards)-1].NumValue == CardAce {
			gotStraight(&cards, player, 3, true)
		}

		if player.Hand.Rank == RankRoyalFlush ||
			player.Hand.Rank == RankStraightFlush {
			return
		}

		if isFlush, _ := gotFlush(cards, player, false); isFlush {
			// replace any previously assembled straight cards before storing flush cards.
			player.Hand.Cards = player.Hand.Cards[:0]
			gotFlush(cards, player, true)
			return
		}

		if player.Hand.Rank == RankStraight {
			return
		}

		// muck
		player.Hand.Rank = RankHighCard
		player.Hand.Cards = append(player.Hand.Cards, cards[bestCard-5],
			cards[bestCard-4], cards[bestCard-3],
			cards[bestCard-2], cards[bestCard-1])

		Assert(len(player.Hand.Cards) == 5, fmt.Sprintf("%d", len(player.Hand.Cards)))

		return
	}

	// quads search //
	if matchHands.quads != nil {
		quadsIdx := int(matchHands.quads[0]) // 0 because it's impossible to
		// get quads twice
		kicker := &Card{}
		for i := bestCard - 1; i >= 0; i-- { // kicker search
			if cards[i].NumValue != cards[quadsIdx].NumValue {
				kicker = cards[i]
				break
			}
		}

		Assert(kicker != nil, "quads: kicker == nil")

		player.Hand.Rank = RankQuads
		player.Hand.Cards = append(Cards{kicker}, cards[quadsIdx:quadsIdx+4]...)

		return
	}

	// fullhouse search //
	//
	// NOTE: we check for a fullhouse before a straight flush because it's
	// impossible to have both at the same time and searching for the fullhouse
	// first saves some cycles+space
	if matchHands.trips != nil && (matchHands.pairs != nil || len(matchHands.trips) > 1) {
		player.Hand.Rank = RankFullHouse

		tripsIdx := int(matchHands.trips[len(matchHands.trips)-1])
		pairIdx := -1

		// Choose the best available pair component for the full house:
		// a second trips can supply the pair, but a higher actual pair should win.
		if len(matchHands.trips) > 1 {
			pairIdx = int(matchHands.trips[len(matchHands.trips)-2])
		}
		if matchHands.pairs != nil {
			highestPairIdx := int(matchHands.pairs[len(matchHands.pairs)-1])
			if pairIdx == -1 || cards[highestPairIdx].NumValue > cards[pairIdx].NumValue {
				pairIdx = highestPairIdx
			}
		}

		Assert(pairIdx != -1, "fullhouse: pairIdx == -1")

		player.Hand.Cards = append(player.Hand.Cards, cards[pairIdx:pairIdx+2]...)
		player.Hand.Cards = append(player.Hand.Cards, cards[tripsIdx:tripsIdx+3]...)

		Assert(len(player.Hand.Cards) == 5, fmt.Sprintf("%d", len(player.Hand.Cards)))

		return
	}

	// flush search //
	//
	// NOTE: we search for the flush here to ease the straight flush logic
	haveFlush, suit := gotFlush(cards, player, false)

	// remove duplicate card (by number) for easy straight search
	uniqueCards := Cards{}

	if haveFlush {
		// check for possible RF/straight flush suit
		cardmap := make(map[CardVal]Suit) // key == num, val == suit

		for _, card := range cards {
			mappedsuit, found := cardmap[card.NumValue]

			if found && mappedsuit != suit && card.Suit == suit {
				cardmap[card.NumValue] = card.Suit
				Assert(uniqueCards[len(uniqueCards)-1].NumValue == card.NumValue, "uniqueCards problem")
				uniqueCards[len(uniqueCards)-1] = card // should _always_ be last card
			} else if !found {
				cardmap[card.NumValue] = card.Suit
				uniqueCards = append(uniqueCards, card)
			}
		}

		Assert((len(uniqueCards) <= 7 && len(uniqueCards) >= 3),
			fmt.Sprintf("impossible number of unique cards (%v)", len(uniqueCards)))
	} else {
		cardmap := make(map[CardVal]bool)

		for _, card := range cards {
			if _, val := cardmap[card.NumValue]; !val {
				cardmap[card.NumValue] = true
				uniqueCards = append(uniqueCards, card)
			}
		}

		Assert((len(uniqueCards) <= 7 && len(uniqueCards) >= 1),
			"impossible number of unique cards")
	}

	// RF, SF and straight search //
	if len(uniqueCards) >= 5 {
		uniqueBestCard := len(uniqueCards)
		iter := uniqueBestCard - 4
		isStraight := false
		//fmt.Printf("iter %v len(uc) %d\n)", iter, len(uniqueCards))

		for i := 1; i <= iter; i++ {
			if gotStraight(&uniqueCards, player, uniqueBestCard-i, false) {
				Assert(len(player.Hand.Cards) == 5, fmt.Sprintf("%d", len(player.Hand.Cards)))
				isStraight = true
				break
			}
		}

		if player.Hand.Rank == RankRoyalFlush ||
			player.Hand.Rank == RankStraightFlush {
			return
		}

		if !isStraight && uniqueCards[uniqueBestCard-1].NumValue == CardAce &&
			gotStraight(&uniqueCards, player, 3, true) {
			Assert(len(player.Hand.Cards) == 5, fmt.Sprintf("%d", len(player.Hand.Cards)))
		}

		if player.Hand.Rank == RankRoyalFlush ||
			player.Hand.Rank == RankStraightFlush {
			return
		}
	}

	if haveFlush {
		// replace any previously assembled straight cards before storing flush cards.
		player.Hand.Cards = player.Hand.Cards[:0]
		gotFlush(cards, player, true)

		Assert(player.Hand.Rank == RankFlush, "player should have a flush")

		return
	}

	if player.Hand.Rank == RankStraight {
		return
	}

	// trips search
	if matchHands.trips != nil {
		firstCard := int(matchHands.trips[len(matchHands.trips)-1])

		tripslice := make(Cards, 0, 3)
		tripslice = append(tripslice, cards[firstCard:firstCard+3]...)

		kickers := top_cards(cards, 2, []CardVal{cards[firstCard].NumValue})
		// order => [kickers][trips]
		kickers = append(kickers, tripslice...)

		player.Hand.Rank = RankTrips
		player.Hand.Cards = kickers

		return
	}

	// two pair & pair search
	if matchHands.pairs != nil {
		if len(matchHands.pairs) > 1 {
			player.Hand.Rank = RankTwoPair
			highPairIdx := int(matchHands.pairs[len(matchHands.pairs)-1])
			lowPairIdx := int(matchHands.pairs[len(matchHands.pairs)-2])

			player.Hand.Cards = append(player.Hand.Cards, cards[lowPairIdx:lowPairIdx+2]...)
			player.Hand.Cards = append(player.Hand.Cards, cards[highPairIdx:highPairIdx+2]...)

			kicker := top_cards(cards, 1, []CardVal{cards[highPairIdx].NumValue,
				cards[lowPairIdx].NumValue})
			player.Hand.Cards = append(kicker, player.Hand.Cards...)
		} else {
			player.Hand.Rank = RankPair
			pairidx := matchHands.pairs[0]
			kickers := top_cards(cards, 3, []CardVal{cards[pairidx].NumValue})
			player.Hand.Cards = append(kickers, cards[pairidx:pairidx+2]...)
		}

		return
	}

	// muck
	player.Hand.Rank = RankHighCard
	player.Hand.Cards = append(player.Hand.Cards, cards[bestCard-5],
		cards[bestCard-4], cards[bestCard-3],
		cards[bestCard-2], cards[bestCard-1])

	return
}
