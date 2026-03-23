package poker

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

var rankCharToVal = map[byte]CardVal{
	'2': CardTwo, '3': CardThree, '4': CardFour, '5': CardFive,
	'6': CardSix, '7': CardSeven, '8': CardEight, '9': CardNine,
	'T': CardTen, 'J': CardJack, 'Q': CardQueen, 'K': CardKing, 'A': CardAce,
}

var suitCharToSuit = map[byte]Suit{
	'c': SuitClub, 'd': SuitDiamond, 'h': SuitHeart, 's': SuitSpade,
}

// mustCard parses a 2-char card code like "Ah" (ace of hearts) or "Td" (ten of diamonds).
// Rank chars: 2-9, T, J, Q, K, A. Suit chars: c, d, h, s.
// cardNumToString populates the Name/FullName display fields on the Card.
func mustCard(t *testing.T, code string) *Card {
	t.Helper()

	if len(code) != 2 {
		t.Fatalf("invalid card code %q", code)
	}

	v, ok := rankCharToVal[code[0]]
	if !ok {
		t.Fatalf("invalid card rank %q in %q", code[0], code)
	}

	s, ok := suitCharToSuit[code[1]]
	if !ok {
		t.Fatalf("invalid card suit %q in %q", code[1], code)
	}

	card := &Card{Suit: s, NumValue: v}
	if err := cardNumToString(card); err != nil {
		t.Fatalf("cardNumToString(%q): %v", code, err)
	}

	return card
}

func mustCards(t *testing.T, cards string) Cards {
	t.Helper()

	if strings.TrimSpace(cards) == "" {
		return nil
	}

	parts := strings.Fields(cards)
	parsed := make(Cards, 0, len(parts))
	for _, part := range parts {
		parsed = append(parsed, mustCard(t, part))
	}

	return parsed
}

// mustPlayerWithHole creates a player with hole cards set up for AssembleBestHand.
// FillHoleInfo must be called so AssembleBestHand can read hole card metadata (e.g. suited, paired).
func mustPlayerWithHole(t *testing.T, name, hole string) *Player {
	t.Helper()

	player := NewPlayer(name, false)
	player.IsVacant = false
	player.Hole.Cards = mustCards(t, hole)
	player.Hole.FillHoleInfo()

	return player
}

// mustPlayerWithHand creates a player with a pre-evaluated hand (rank + cards already known).
// Used by TestBestHandRanksAllCategories to test BestHand comparison without going through AssembleBestHand.
func mustPlayerWithHand(t *testing.T, name string, rank Rank, cards string) *Player {
	t.Helper()

	player := NewPlayer(name, false)
	player.IsVacant = false
	player.Hand = &Hand{
		Rank:  rank,
		Cards: mustCards(t, cards),
	}

	return player
}

func newTestTable(t *testing.T, community string) *Table {
	t.Helper()

	return &Table{
		Community: mustCards(t, community),
		State:     TableStateRiver,
	}
}

func handValues(cards Cards) []CardVal {
	values := make([]CardVal, 0, len(cards))
	for _, card := range cards {
		values = append(values, card.NumValue)
	}

	return values
}

func winnerNames(players []*Player) []string {
	names := make([]string, 0, len(players))
	for _, player := range players {
		names = append(names, player.Name)
	}

	return names
}

type assembleBestHandCase struct {
	name      string
	community string
	hole      string
	wantRank  Rank
	wantCards []CardVal
}

// runAssembleBestHandCases runs AssembleBestHand for each case and checks both the
// resulting rank and the exact cards in the best 5-card hand.
// wantCards order must match the hand's internal ordering: kickers first, then the
// ranked cards (e.g. for a pair: [kicker kicker kicker pairCard pairCard]).
func runAssembleBestHandCases(t *testing.T, tests []assembleBestHandCase) {
	t.Helper()
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			table := newTestTable(t, tt.community)
			player := mustPlayerWithHole(t, "p1", tt.hole)

			AssembleBestHand(false, table, player)

			if player.Hand.Rank != tt.wantRank {
				t.Fatalf("rank mismatch: got %s (%d), want %s (%d)",
					player.Hand.RankName(), player.Hand.Rank, (&Hand{Rank: tt.wantRank}).RankName(), tt.wantRank)
			}

			if got := handValues(player.Hand.Cards); !reflect.DeepEqual(got, tt.wantCards) {
				t.Fatalf("hand cards mismatch: got %v, want %v", got, tt.wantCards)
			}
		})
	}
}

func TestAssembleBestHandRanks(t *testing.T) {
	runAssembleBestHandCases(t, []assembleBestHandCase{
		{
			name:      "high card",
			community: "2c 5d 9h Jc Kd",
			hole:      "3s 7h",
			wantRank:  RankHighCard,
			wantCards: []CardVal{CardFive, CardSeven, CardNine, CardJack, CardKing},
		},
		{
			name:      "pair",
			community: "2c 5d 9h Jc Kd",
			hole:      "5s 7h",
			wantRank:  RankPair,
			wantCards: []CardVal{CardNine, CardJack, CardKing, CardFive, CardFive},
		},
		{
			name:      "two pair",
			community: "2c 2d 9h Jc Kd",
			hole:      "9s 7h",
			wantRank:  RankTwoPair,
			wantCards: []CardVal{CardKing, CardTwo, CardTwo, CardNine, CardNine},
		},
		{
			name:      "three of a kind",
			community: "2c 5d 9h 9c Kd",
			hole:      "9s 7h",
			wantRank:  RankTrips,
			wantCards: []CardVal{CardSeven, CardKing, CardNine, CardNine, CardNine},
		},
		{
			name:      "straight with duplicate ranks available",
			community: "5c 6d 7h 8c Kd",
			hole:      "9s 7s",
			wantRank:  RankStraight,
			wantCards: []CardVal{CardFive, CardSix, CardSeven, CardEight, CardNine},
		},
		{
			name:      "flush",
			community: "2h 5h 9h Jh Kd",
			hole:      "7h Ah",
			wantRank:  RankFlush,
			wantCards: []CardVal{CardFive, CardSeven, CardNine, CardJack, CardAce},
		},
		{
			name:      "full house",
			community: "Kc Kd Kh 9c 2d",
			hole:      "9h As",
			wantRank:  RankFullHouse,
			wantCards: []CardVal{CardNine, CardNine, CardKing, CardKing, CardKing},
		},
		{
			name:      "four of a kind",
			community: "9c 9d 9h Kc 2d",
			hole:      "9s As",
			wantRank:  RankQuads,
			wantCards: []CardVal{CardAce, CardNine, CardNine, CardNine, CardNine},
		},
		{
			name:      "straight flush",
			community: "5h 6h 7h 8h Kd",
			hole:      "9h As",
			wantRank:  RankStraightFlush,
			wantCards: []CardVal{CardFive, CardSix, CardSeven, CardEight, CardNine},
		},
		{
			name:      "royal flush",
			community: "Th Jh Qh Kh 2d",
			hole:      "Ah 3s",
			wantRank:  RankRoyalFlush,
			wantCards: []CardVal{CardTen, CardJack, CardQueen, CardKing, CardAce},
		},
	})
}

func TestAssembleBestHandEdgeCases(t *testing.T) {
	runAssembleBestHandCases(t, []assembleBestHandCase{
		{
			name:      "wheel straight",
			community: "2c 3d 4h 8c Kd",
			hole:      "5s As",
			wantRank:  RankStraight,
			wantCards: []CardVal{CardAce, CardTwo, CardThree, CardFour, CardFive},
		},
		{
			name:      "double trips become full house",
			community: "Kc Kd Kh 9c 9d",
			hole:      "9h 2s",
			wantRank:  RankFullHouse,
			wantCards: []CardVal{CardNine, CardNine, CardKing, CardKing, CardKing},
		},
		{
			name:      "board-made royal flush",
			community: "Th Jh Qh Kh Ah",
			hole:      "2c 3d",
			wantRank:  RankRoyalFlush,
			wantCards: []CardVal{CardTen, CardJack, CardQueen, CardKing, CardAce},
		},
		{
			name:      "wheel straight flush",
			community: "2h 3h 4h Kd Qc",
			hole:      "5h Ah",
			wantRank:  RankStraightFlush,
			wantCards: []CardVal{CardAce, CardTwo, CardThree, CardFour, CardFive},
		},
		{
			name:      "wheel straight with duplicate ranks",
			community: "2c 2d 3h 4s Kd",
			hole:      "5c As",
			wantRank:  RankStraight,
			wantCards: []CardVal{CardAce, CardTwo, CardThree, CardFour, CardFive},
		},
		{
			name:      "wheel straight flush with duplicate ranks",
			community: "2h 2c 3h 4h Kd",
			hole:      "5h Ah",
			wantRank:  RankStraightFlush,
			wantCards: []CardVal{CardAce, CardTwo, CardThree, CardFour, CardFive},
		},
		{
			name:      "wheel straight upgraded to flush replaces cards",
			community: "3s 4d 5c 9s Ks",
			hole:      "As 2s",
			wantRank:  RankFlush,
			wantCards: []CardVal{CardTwo, CardThree, CardNine, CardKing, CardAce},
		},
		{
			name:      "wheel straight with duplicate ranks upgraded to flush replaces cards",
			community: "2d 3s 4s 5h 9s",
			hole:      "As 2s",
			wantRank:  RankFlush,
			wantCards: []CardVal{CardTwo, CardThree, CardFour, CardNine, CardAce},
		},
	})
}

func TestBestHandRanksAllCategories(t *testing.T) {
	fixtures := []struct {
		name  string
		rank  Rank
		cards string
	}{
		{name: "high card", rank: RankHighCard, cards: "5c 7d 9h Js Kc"},
		{name: "pair", rank: RankPair, cards: "9c Jd Kh 5s 5d"},
		{name: "two pair", rank: RankTwoPair, cards: "Kc 2d 2h 9s 9d"},
		{name: "trips", rank: RankTrips, cards: "7c Kd 9h 9s 9d"},
		{name: "straight", rank: RankStraight, cards: "5c 6d 7h 8s 9d"},
		{name: "flush", rank: RankFlush, cards: "5h 7h 9h Jh Ah"},
		{name: "full house", rank: RankFullHouse, cards: "9c 9d Kh Ks Kd"},
		{name: "quads", rank: RankQuads, cards: "Ac 9d 9h 9s 9c"},
		{name: "straight flush", rank: RankStraightFlush, cards: "5h 6h 7h 8h 9h"},
		{name: "royal flush", rank: RankRoyalFlush, cards: "Th Jh Qh Kh Ah"},
	}

	// Compare every pair of fixtures to verify rank ordering is transitive:
	// every lower-ranked hand must lose to every higher-ranked hand (45 sub-tests).
	for lowerIdx := 0; lowerIdx < len(fixtures); lowerIdx++ {
		for higherIdx := lowerIdx + 1; higherIdx < len(fixtures); higherIdx++ {
			lower := fixtures[lowerIdx]
			higher := fixtures[higherIdx]

			name := fmt.Sprintf("%s loses to %s", lower.name, higher.name)
			t.Run(name, func(t *testing.T) {
				table := &Table{}
				// Pass a non-nil side pot so BestHand compares the prebuilt Hand values
				// directly instead of trying to assemble/log from missing hole cards.
				sidePot := NewSidePot(0)
				players := []*Player{
					mustPlayerWithHand(t, "lower", lower.rank, lower.cards),
					mustPlayerWithHand(t, "higher", higher.rank, higher.cards),
				}

				winners := table.BestHand(players, sidePot)
				if got := winnerNames(winners); !reflect.DeepEqual(got, []string{"higher"}) {
					t.Fatalf("winner mismatch: got %v, want [higher]", got)
				}
			})
		}
	}
}

// TestBestHandTieBreakers tests BestHand winner selection when both players have
// the same rank. Verifies kicker logic, split pots, and cases where a seemingly
// strong hand (e.g. wheel straight) gets upgraded to a different rank (e.g. flush).
func TestBestHandTieBreakers(t *testing.T) {
	tests := []struct {
		name      string
		community string
		players   []*Player
		want      []string
	}{
		{
			name:      "high card uses next kicker",
			community: "Ac Jd 9h 5c 2d",
			players:   []*Player{mustPlayerWithHole(t, "alice", "Kh 7s"), mustPlayerWithHole(t, "bob", "Qh 8s")},
			want:      []string{"alice"},
		},
		{
			name:      "pair uses kickers after pair rank",
			community: "5c 5d 9h Jc 2d",
			players:   []*Player{mustPlayerWithHole(t, "alice", "Ah Kc"), mustPlayerWithHole(t, "bob", "Ad Qc")},
			want:      []string{"alice"},
		},
		{
			name:      "two pair compares second pair before kicker",
			community: "Kc Kd 5h 2c 9d",
			players:   []*Player{mustPlayerWithHole(t, "alice", "Ah 5s"), mustPlayerWithHole(t, "bob", "As 2s")},
			want:      []string{"alice"},
		},
		{
			name:      "trips compares kickers",
			community: "9c 9d 9h 2c 3d",
			players:   []*Player{mustPlayerWithHole(t, "alice", "As Kc"), mustPlayerWithHole(t, "bob", "Ad Qc")},
			want:      []string{"alice"},
		},
		{
			name:      "higher straight wins",
			community: "2c 3d 4h 5s Kd",
			players:   []*Player{mustPlayerWithHole(t, "alice", "As Qc"), mustPlayerWithHole(t, "bob", "6c Qd")},
			want:      []string{"bob"},
		},
		{
			name:      "higher flush wins",
			community: "2h 5h 9h Jh Kd",
			players:   []*Player{mustPlayerWithHole(t, "alice", "Ah 3h"), mustPlayerWithHole(t, "bob", "Qh 4h")},
			want:      []string{"alice"},
		},
		{
			name:      "full house compares trips before pair",
			community: "Kc Kd 9h 2c 2d",
			players:   []*Player{mustPlayerWithHole(t, "alice", "Kh As"), mustPlayerWithHole(t, "bob", "9d 9s")},
			want:      []string{"alice"},
		},
		{
			name:      "quads uses kicker",
			community: "9c 9d 9h 9s 2d",
			players:   []*Player{mustPlayerWithHole(t, "alice", "As Kc"), mustPlayerWithHole(t, "bob", "Qh Jc")},
			want:      []string{"alice"},
		},
		{
			name:      "higher straight flush wins",
			community: "5h 6h 7h 8h 2d",
			players:   []*Player{mustPlayerWithHole(t, "alice", "9h As"), mustPlayerWithHole(t, "bob", "4h Kc")},
			want:      []string{"alice"},
		},
		{
			name:      "board-made royal flush splits",
			community: "Th Jh Qh Kh Ah",
			players:   []*Player{mustPlayerWithHole(t, "alice", "2c 3d"), mustPlayerWithHole(t, "bob", "4c 5d")},
			want:      []string{"alice", "bob"},
		},
		{
			name:      "board-made straight splits",
			community: "6h 7d 8c 9s Td",
			players:   []*Player{mustPlayerWithHole(t, "alice", "Ac 2d"), mustPlayerWithHole(t, "bob", "Kh 3c")},
			want:      []string{"alice", "bob"},
		},
		{
			name:      "two pair board plus both ranks in hole becomes full house",
			community: "2d 2s 3c 3h 4s",
			players:   []*Player{mustPlayerWithHole(t, "b", "7c 5c"), mustPlayerWithHole(t, "b2", "2c 3d")},
			want:      []string{"b2"},
		},
		{
			name:      "flush beats lower flush even when winner also has wheel straight",
			community: "3s 4d 5c 9s Ks",
			players:   []*Player{mustPlayerWithHole(t, "alice", "As 2s"), mustPlayerWithHole(t, "bob", "Qs Js")},
			want:      []string{"alice"},
		},
		{
			name:      "duplicate-rank wheel flush uses flush cards for tie break",
			community: "2d 3s 4s 5h 9s",
			players:   []*Player{mustPlayerWithHole(t, "alice", "As 2s"), mustPlayerWithHole(t, "bob", "Ks Qs")},
			want:      []string{"alice"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			table := newTestTable(t, tt.community)
			winners := table.BestHand(tt.players, nil)
			if got := winnerNames(winners); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("winner mismatch: got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAssembleBestHandPreShowDoesNotMutateExistingHand(t *testing.T) {
	table := newTestTable(t, "3s 4d 5c 9s Ks")
	player := mustPlayerWithHole(t, "preview", "As 2s")
	player.Hand = &Hand{
		Rank:  RankPair,
		Cards: mustCards(t, "2c 2d Ah Kc Qd"),
	}

	AssembleBestHand(true, table, player)

	if player.Hand.Rank != RankPair {
		t.Fatalf("restored rank mismatch: got %s (%d), want %s (%d)",
			player.Hand.RankName(), player.Hand.Rank, (&Hand{Rank: RankPair}).RankName(), RankPair)
	}

	if got := handValues(player.Hand.Cards); !reflect.DeepEqual(got,
		[]CardVal{CardTwo, CardTwo, CardAce, CardKing, CardQueen}) {
		t.Fatalf("restored hand cards mismatch: got %v", got)
	}

	if player.preHand == nil || player.preHand.Rank != RankFlush {
		t.Fatalf("preview hand rank mismatch: got %#v, want flush", player.preHand)
	}

	if got := handValues(player.preHand.Cards); !reflect.DeepEqual(got,
		[]CardVal{CardTwo, CardThree, CardNine, CardKing, CardAce}) {
		t.Fatalf("preview hand cards mismatch: got %v", got)
	}
}
