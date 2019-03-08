#! /usr/bin/env perl

use strict;
use vars q($h);
use warnings;

use POSIX qw(strftime);
use Storable;

$h = retrieve('md5-history.dat')
	or die "Unable to load md5-history.dat: $!";

sub commit_range {
	my ($p, $i, $md5) = @_;
	my $e = $p->[$i];
	return () unless $e->{md5} eq $md5;

	my $commit = $e->{commit};
	return ($commit, '') if $i == 0;

	my $succ = $p->[$i-1]->{commit};
	my $nm1 = $h->{commits}->{$succ}->{parents}->[0];
	return ($commit, $nm1);
}

sub search {
	my ($path, $md5) = @_;

	# Qualify the file path.
	if (exists $h->{paths}->{$path}) {
		# the file exists
	} elsif ($path =~ /\.h$/ and exists $h->{paths}->{"include/" . $path}) {
		$path = "include/" . $path;
	} elsif (exists $h->{paths}->{"ircd/" . $path}) {
		$path = "ircd/" . $path;
	} else {
		die "Don't know where to find $path\n";
	}

	# Find commit ranges with the requested md5sum.
	my $p = $h->{paths}->{$path} or die "Unknown path $path\n";
	my @res = ($path);
	for my $i (0..$#$p) {
		push @res, commit_range($p, $i, $md5);
	}
	return @res;
}

sub commit_time {
	return strftime("%F %T", gmtime($h->{commits}->{$_[0]}->{time}));
}

sub report {
	my $path = shift;
	while (@_) {
		my $base = shift;
		my $start = commit_time($base);
		$base = substr($base, 0, 8);

		my $next = shift;
		my $end = 'now';
		if ($next) {
			$end = commit_time($next);
			$next = substr($next, 0, 8);
		} else {
			$next = 'current';
		}

		print "$path: $start .. $end ($base .. $next)\n";
	}
}

# is_ancestor returns 1 iff $a is an ancestor of $b.
sub is_ancestor {
	my $a = shift; # remaining args are in queue
	my %v; # maps visited commit IDs to 1

	while (@_) {
		$b = pop;
		next if $v{$b};
		return 1 if $a eq $b;
		$v{$b} = 1;
		push @_, @{$h->{commits}->{$b}->{parents}}
			if $h->{commits}->{$b}->{parents};
	}

	return 0;
}

sub update {
	my ($aref, $base, $next) = @_;
	$next ||= $h->{HEAD};

	# If this commit range overlaps with an existing one, replace
	# the existing one with the intersection of the two ranges.
	for my $inner (@$aref) {
		next unless is_ancestor($base, $inner->[1])
			and is_ancestor($inner->[0], $next);

		$inner->[0] = $base if is_ancestor($inner->[0], $base);
		$inner->[1] = $next if is_ancestor($next, $inner->[1]);
		return;
	}

	# Add this as a new commit range.
	push @$aref, [$base, $next];
}

sub consistent {
	my ($beg, $end, $href) = @_;
	my @c;
	while (my ($k, $v) = each %$href) {
		my $okay;
		for my $aref (@$v) {
			$okay = 1 if is_ancestor($v->[0], $beg)
				and ($v->[1] ? is_ancestor($end, $v->[1]) : 1);
			last if $okay;
		}
		push @c, $k if $okay;
	}
	return @c;
}

if ($#ARGV < 0) {
	# Each element of @spans is an array ref of [begin, end] commits.
	my @spans;
	my %ranges;

	# Construct list of plausible commit spans.
	while (<>) {
		# The uncaptured .* used to be a CVS $Id: <version>$.
		next unless /\[ ([^:]+): ([0-9a-f]{32}) .* \]/;
		my ($path, $md5) = ($1, $2);
		next if $path eq "patchlist.h";
		my @r = search($path, $md5);
		shift @r; # discard corrected filename
		if (not @r) {
			print "$path: $md5 not known\n";
			continue;
		}

		# Record ranges for this path and update @spans.
		$ranges{$path} = \@r;
		for (my $i = 0; $i < $#r; $i += 2) {
			my $base = $r[$i+0];
			my $next = $r[$i+1];
			update(\@spans, $base, $next);

			my $b_time = commit_time($base);
			my $b_abbrev = substr($base, 0, 8);
		}
	}

	# Report each commit range, along with which files are (or are
	# not) consistent with it.
	my $t = scalar keys %ranges;
	for my $span (@spans) {
		# Report the commit(s) in this span.
		my ($first, $last) = @$span;
		my $start = commit_time($first);
		my $f_abbrev = substr($first, 0, 8);
		if ($first eq $last) {
			print "$f_abbrev ($start)";
		} else {
			my $end = commit_time($last);
			my $l_abbrev = substr($last, 0, 8);
			print "$f_abbrev .. $l_abbrev ($start .. $end)";
		}

		# Which files have the right MD5sum for this range?
		my @c = consistent($first, $last, \%ranges);
		my $n = scalar @c;
		print ", $n/$t consistent";
		if (2*$n > $t) {
			# mostly consistent, report exceptions
			print "; all except" if $n < $t;
			my %c = map {$_ => 1} @c;
			for my $path (keys %ranges) {
				next if $c{$path};
				print " $path";
			}
		} else {
			print ":" if $n > 0;
			# mostly inconsistent, report which files matched
			for my $path (@c) {
				print " $path";
			}
		}
		print "\n";
	}
} elsif ($#ARGV == 0) {
	my ($md5) = @ARGV;
	my $hit = 0;
	while (my ($path, $p) = each %{$h->{paths}}) {
		my @r = search($path, $md5);
		next unless @r;
		$hit = 1;
		report(@r);
	}
	print "No $md5 found in any file\n" unless $hit;
} elsif ($#ARGV == 1) {
	my ($md5, $filter) = @ARGV;
	my @r = search($filter, $md5);
	if (@r) {
		report(@r);
	} else {
		print "No $md5 found in $filter\n";
	}
} else {
	die "Unknown number of arguments.  Usage: $0 [md5sum [filename]]\n"
}