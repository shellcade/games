//! The game declares a "Wins" leaderboard so the arcade records each match.
//! Driven through the game's own `meta()` via the SDK-generated
//! `__shellcade_game()` constructor (the native-build handle the
//! `shellcade_game!` macro emits).

use shellcade_kit::{Aggregation, Direction, Game, MetricFormat};
use tic_tac_toe_rs::__shellcade_game;

#[test]
fn meta_declares_a_wins_leaderboard() {
    let lb = __shellcade_game()
        .meta()
        .leaderboard
        .expect("Meta must declare a leaderboard so results are recorded");
    assert_eq!(lb.metric_label, "Wins");
    assert_eq!(lb.direction, Direction::HigherBetter);
    // Cumulative win count across every match the player has played.
    assert_eq!(lb.aggregation, Aggregation::SumResults);
    assert_eq!(lb.format, MetricFormat::Integer);
}
