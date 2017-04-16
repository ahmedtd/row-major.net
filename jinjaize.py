import argparse
import base64
import os.path
import sys

import csscompressor
import htmlmin
import jinja2
import jinja2.environment
import jinja2.ext

class CSSBlockExtension(jinja2.ext.Extension):
    tags = set(['jinjaizecss'])
    def __init__(self, environment):
        super(CSSBlockExtension, self).__init__(environment)
    def parse(self, parser):
        lineno = next(parser.stream).lineno
        body = parser.parse_statements(['name:endjinjaizecss'], drop_needle=True)
        return jinja2.nodes.CallBlock(self.call_method('minify'), [], [], body).set_lineno(lineno)
    def minify(self, caller):
        return csscompressor.compress(caller())

def file_to_datauri(template_path, name, mime):
    with open(os.path.join(template_path, name), 'rb') as input_file:
        b64 = base64.b64encode(input_file.read()).decode('utf-8')
        return 'data:{};base64,{}'.format(mime, b64)

def main():
    """ Entry point for the package, as defined in `setup.py`. """

    parser = argparse.ArgumentParser()
    parser.add_argument('--template-path', default='./')
    args = parser.parse_args()

    env = jinja2.environment.Environment(extensions=[CSSBlockExtension])
    env.loader = jinja2.FileSystemLoader(args.template_path)

    env.globals['file_to_datauri'] = lambda name, mime: file_to_datauri(args.template_path,
                                                                        name,
                                                                        mime)

    # Read stdin to string
    tpl = env.from_string(sys.stdin.read())
    sys.stdout.write(htmlmin.minify(tpl.render()))

if __name__ == '__main__':
    main()
