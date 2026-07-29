import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "scripts"))

from lib.buckets import classify_domain, classify_rule, empty_buckets, glob_to_regex
from lib.dlc_emit import buckets_to_dlc_lines, buckets_to_ip_lines


class TestDomainBehavior(unittest.TestCase):
    def test_suffix_and_exact_are_prefixed(self):
        buckets = empty_buckets()
        classify_domain("+.example.com", buckets)
        classify_domain("www.example.com", buckets)

        lines = buckets_to_dlc_lines(buckets)

        self.assertIn("domain:example.com", lines)
        self.assertIn("full:www.example.com", lines)
        self.assertNotIn("example.com", lines)

    def test_keyword_classical(self):
        buckets = empty_buckets()
        skipped = []
        classify_rule("DOMAIN-KEYWORD,openai", buckets, skipped)

        self.assertIn("keyword:openai", buckets_to_dlc_lines(buckets))

    def test_process_name_is_skipped(self):
        buckets = empty_buckets()
        skipped = []
        classify_rule("PROCESS-NAME,Chrome", buckets, skipped)

        self.assertEqual(skipped, ["PROCESS-NAME,Chrome"])
        self.assertEqual(sum(len(values) for values in buckets.values()), 0)

    def test_netflix_splits_domain_and_ip_buckets(self):
        buckets = empty_buckets()
        skipped = []
        classify_rule("DOMAIN-SUFFIX,netflix.com", buckets, skipped)
        classify_rule("IP-CIDR,23.246.0.0/18,no-resolve", buckets, skipped)

        self.assertIn("domain:netflix.com", buckets_to_dlc_lines(buckets))
        self.assertEqual(buckets_to_ip_lines(buckets), ["23.246.0.0/18"])

    def test_glob_regex_is_anchored_and_dot_safe(self):
        regex = glob_to_regex("foo*bar?.com")

        self.assertTrue(regex.startswith("^") and regex.endswith("$"))
        self.assertIn("[^.]*", regex)
        self.assertIn("[^.]", regex)
        self.assertIn(r"\.com", regex)


if __name__ == "__main__":
    unittest.main()
