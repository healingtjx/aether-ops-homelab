"""验证测试环境发布配置；不连接服务器，不执行真实构建。"""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / 'doc/scripts/tanliao-server/tanliao-server-测试环境部署.sh'


class TanliaoServerDeployTest(unittest.TestCase):
    def test_parent_uses_java8_dependency_family(self):
        code = SCRIPT.read_text().split("<<'JDK8_PARENT'\n", 1)[1].split('\nJDK8_PARENT', 1)[0]
        original = ('<project xmlns="http://maven.apache.org/POM/4.0.0">'
                    '<parent><artifactId>tio-zoo-parent</artifactId>'
                    '<version>3.8.6.v20240801-RELEASE</version></parent>'
                    '<properties><maven.compiler.target>1.8</maven.compiler.target>'
                    '</properties></project>')
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            (root / 'parent').mkdir()
            pom = root / 'parent/pom.xml'
            pom.write_text(original)
            for _ in range(2):
                subprocess.run(['python3', '-'], input=code, cwd=folder,
                               text=True, check=True, capture_output=True)
                self.assertIn('<version>3.8.6.v20240801-tanliao-jdk8-1</version>', pom.read_text())
            pom.write_text(original.replace('3.8.6.v20240801-RELEASE', 'unknown'))
            result = subprocess.run(['python3', '-'], input=code, cwd=folder,
                                    text=True, capture_output=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn('<version>unknown</version>', pom.read_text())

    def test_dev_properties_override_source_ports(self):
        script = SCRIPT.read_text()
        code = script.split("<<'PROPERTIES'\n", 1)[1].split('\nPROPERTIES', 1)[0]
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            (root / 'app.properties').write_text('http.api.port = 6060\nim.port.ws = 9325\n')
            (root / 'app-dev.properties').write_text(
                'http.api.port=6060\nim.port.ws=9325\nim.port.app=9326\n'
                'site=http://192.168.5.4:8085\nhttp.cookie.domain=old.example\n'
                'tio_site_main.jdbc.url=jdbc:mysql://database/tio_site_main\n')
            (root / 'logback.properties').write_text('log.dir=/tio/logs/tio-site\n')
            subprocess.run(['python3', '-', folder, '21008', '21010', '21011',
                            '21012', '192.168.5.4', '21009'], input=code,
                           text=True, check=True)
            values = {}
            for name in ('app.properties', 'app-dev.properties'):
                for line in (root / name).read_text().splitlines():
                    if '=' in line and not line.startswith('#'):
                        key, value = line.split('=', 1)
                        values[key.strip()] = value.strip()
            self.assertEqual({key: values[key] for key in (
                'http.api.port', 'im.port.ws', 'im.port.app', 'http.view.port',
                'site', 'http.cookie.domain', 'tio_site_main.jdbc.url')}, {
                    'http.api.port': '21008', 'im.port.ws': '21010',
                    'im.port.app': '21011', 'http.view.port': '21012',
                    'site': 'http://192.168.5.4:21009', 'http.cookie.domain': '',
                    'tio_site_main.jdbc.url': 'jdbc:mysql://database/tio_site_main'})

    def test_frontend_build_uses_test_environment(self):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            (root / 'bin').mkdir()
            (root / 'pc-html').mkdir()
            (root / 'pc-html/package.json').write_text(
                '{"devDependencies": {\n'
                '    "webpack-dev-server": "3.11.3",\n'
                '    "@vue/cli-service": "5.0.6"\n}}')
            (root / 'pc-html/package-lock.json').write_text(
                '{"packages":{"":{"devDependencies":{"@vue/cli-service":"^4.5.0"}}}}')
            mocks = {
                'ssh': 'cat >/dev/null\ncase "$*" in *mktemp*) echo /tmp/tanliao-server.mock1234;; esac\n',
                'scp': 'exit 0\n',
                'node': 'echo 20\n',
                'npm': 'if [[ "$1" == --version ]]; then echo 10.8.2; fi\n'
                       'if [[ "$1" == ci ]]; then\n'
                       'python3 -c "import json; assert json.load(open(\'package.json\'))[\'devDependencies\'] == json.load(open(\'package-lock.json\'))[\'packages\'][\'\'][\'devDependencies\']"\n'
                       'fi\n'
                       'if [[ "$1" == run ]]; then\n'
                       '[[ "$2" == build && "$VUE_APP_apiCtx" == /mytio ]]\n'
                       '[[ "$VUE_APP_wsCtx" == ws://192.168.5.4:21010 ]]\n'
                       'mkdir dist; echo test > dist/index.html\nfi\n',
            }
            for name, content in mocks.items():
                path = root / 'bin' / name
                path.write_text('#!/usr/bin/env bash\nset -euo pipefail\n' + content)
                path.chmod(0o755)
            env = {key: value for key, value in os.environ.items()
                   if not key.startswith(('REMOTE_', 'SERVER_', 'WS_', 'APP_',
                                          'VIEW_', 'BACKEND_', 'FRONTEND_',
                                          'DEPLOY_', 'PUBLIC_', 'API_BASE_'))}
            env.update(PATH=str(root / 'bin') + ':' + env['PATH'],
                       MODULE_NAME='pc-html', REMOTE_HOST='192.168.5.4')
            result = subprocess.run(['bash', str(SCRIPT)], cwd=folder, env=env,
                                    capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertFalse((root / '.tanliao-server-build.lock').exists())

    def test_sql_embeds_current_script(self):
        sql = (ROOT / 'doc/sql/ci_config_tanliao_server_test.sql').read_text()
        self.assertEqual(sql.split('$tanliao_server_ci$')[1], SCRIPT.read_text())


if __name__ == '__main__':
    unittest.main()
