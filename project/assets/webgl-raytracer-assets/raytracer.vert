// A do-nothing vertex shader.  We only ever draw one rectangle, to
// get a nice surface for our fragment shader raytracer.

attribute vec2 vertex_pos;
void main()
{
    gl_Position = vec4(vertex_pos, 0.0, 1.0);
}
