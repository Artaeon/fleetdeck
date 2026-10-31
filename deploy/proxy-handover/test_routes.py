import unittest
from routes import render, ipv6_input_rules


class RulesTest(unittest.TestCase):
    def setUp(self):
        self.args = ["eth0", "192.0.2.10", "2001:db8::10", "172.20.0.8", 18080, 18443]

    def test_scope_and_original_destination(self):
        text = render(*self.args)
        self.assertNotIn("flush ruleset", text)
        self.assertNotIn("DOCKER", text)
        self.assertIn("priority -101", text)
        self.assertIn('iifname "eth0" ip daddr 192.0.2.10', text)
        for rule in ipv6_input_rules(*self.args):
            self.assertIn("--ctorigdstport", rule)
            self.assertIn("--ctstate", rule)
            self.assertIn("DNAT", rule)

    def test_invalid_scope_never_renders(self):
        for position, value in [(0, 'eth0; flush ruleset'), (1, '::1'), (1, '0.0.0.0'),
                                (2, 'ff02::1'), (3, '8.8.8.8'), (3, '127.0.0.1'),
                                (4, 22), (4, 18443), (5, '18443'), (5, True)]:
            args = self.args[:]
            args[position] = value
            with self.subTest(position=position, value=value), self.assertRaises(ValueError):
                render(*args)


if __name__ == "__main__":
    unittest.main()
