import copy
import unittest
from bridge_guard import render

class PolicyValidation(unittest.TestCase):
    def setUp(self):
        self.ports = [dict(port='vethproxy', mac='02:00:00:00:00:01', ip='198.18.0.1', group='proxy', proxy=True),
                      dict(port='vethapp', mac='02:00:00:00:00:02', ip='198.18.0.2', group='tenant', proxy=False)]

    def test_rejects_rule_injection(self):
        for field, value in [('port','veth0" accept; #'),('mac','00:00:00:00:00:00 accept'),('ip','198.18.0.2 accept'),('group','a\nb')]:
            with self.subTest(field=field):
                p=copy.deepcopy(self.ports);p[1][field]=value
                with self.assertRaises(ValueError):render('br-test',p)
        with self.assertRaises(ValueError):render('br-test" accept',self.ports)

    def test_rejects_ambiguous_or_unreviewed_inventory(self):
        for p in [[],self.ports[1:],self.ports+self.ports]:
            with self.assertRaises(ValueError):render('br-test',p)
        p=copy.deepcopy(self.ports);p[0]['proxy']='true'
        with self.assertRaises(ValueError):render('br-test',p)

    def test_ipv6_inventory_requires_separate_design(self):
        p=copy.deepcopy(self.ports);p[1]['ip']='2001:db8::1'
        with self.assertRaises(ValueError):render('br-test',p)

if __name__=='__main__':unittest.main()
