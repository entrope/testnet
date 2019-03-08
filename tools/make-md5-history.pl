#! /usr/bin/env perl

use strict;
use vars qw($h $last_commit);
use warnings;
use Digest::MD5 qw(md5_hex);
use Storable;

# Initialize the variable we will save to disk.
$h = {
	# commits maps (hex-coded) hashes to { time => $unixts, parents => [ hash... ] }
	commits => {},
	# paths maps path names to { $md5sum => $commitid }
	paths => {},
};

# Build a list of commits, from current backwards.
my @commits;
open(my $fh, "-|", "git", "log", "--topo-order", "--format=%H %ct %P")
	or die "Can't run git log: $!";
while (<$fh>) {
	if (/^([0-9a-f]{40}) (\d+) ?(.*)$/) {
		$h->{HEAD} = $1 unless $h->{HEAD};
		push @commits, [$1, $2, split(' ', $3)];
	}
}
close($fh);

# Given a git commit id, record any changed files in it.
sub load_commit {
	my ($hash) = @_;
	open(my $fh, "-|", "git", "show", "--pretty=oneline", "--name-status", $hash);
	my $summary = <$fh>;
	while (<$fh>) {
		my $path;
		if (/^D\t.*$/) {
			next;
		} elsif (/^[AM]\t(.*)$/) {
			$path = $1;
		} elsif (/^R\d\d\d\t(.*)\t(.*)$/) {
			my $old = $1;
			$path = $2;
			push @{$h->{paths}->{$old}}, { commit => $hash, md5 => '' };
			$path = $2;
		} else {
			die "Unmatched oneline status: $_";
		}

		my $md5 = md5_hex(`git show $hash:$path`);
		push @{$h->{paths}->{$path}}, { commit => $hash, md5 => $md5 };
	}
	close($fh);
}

# For each commit, update the files it touched.
for my $aref (@commits) {
	my ($hash, $unixts, @parents) = @$aref;
	$h->{commits}->{$hash} = {
		time => $unixts,
		parents => \@parents,
	};
	load_commit($hash, $unixts);
}

# Save our data.
store $h, 'md5-history.dat';
