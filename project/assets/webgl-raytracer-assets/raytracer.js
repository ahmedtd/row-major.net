// The canvas into which we draw.
var canvas;

// The WebGL context.
var gl;
var glext_oes_texture_float;

// Vertices.
var vertex_buffer;

// The raytracing shader program.
var rt_prog;

// Vertex attribute from rt_prog.
var rt_prog_vertex_pos;

// Uniforms from rt_prog.
var rt_prog_viewport;
var rt_prog_cs_to_ws_linear;
var rt_prog_cs_to_ws_offset;
var rt_prog_cam_aperture;

// Texture used to stuff scene description into the fragment shader.
var rt_prog_scenedesc;
var rt_prog_scenedesc_dim;
var rt_prog_scenedesc_tex;

// Texture used to stuff material descriptions into the fragment shader.
var rt_prog_materialdesc;     // The sampler uniform.
var rt_prog_materialdesc_dim; // The texture dimension.
var rt_prog_materialdesc_tex; // The texture object.

var material_code_fragments;
var material_code;

function rt_prog_encode_int(val)
{
    return val / 65536.0;
}

function rt_prog_encode_float(val)
{
    return (val / 65536.0) / 2.0 + (1.0/2.0);
}

function rt_prog_floatcode_material_constant(color)
{
    var constant_code = [
        rt_prog_encode_int(1),
        rt_prog_encode_int(0),
        color[0],
        color[1],
        color[2]
    ];

    return constant_code;
}

function rt_prog_floatcode_material_lambert(base_color, ambient, light_pos)
{
    var lambert_code = [
        rt_prog_encode_int(1),     // Define material.
        rt_prog_encode_int(1),     //     type: 1 (material_lambert)
        base_color[0],             //     rgba base color
        base_color[1],
        base_color[2],
        ambient,                             //     base illuminance.
        rt_prog_encode_float(light_pos[0]),  //     point light position.
        rt_prog_encode_float(light_pos[1]),
        rt_prog_encode_float(light_pos[2])
    ];

    return lambert_code;
}

function floatcode_write(texture_data, program_bits)
{
    var cur_offset = 0;
    for(var i in program_bits)
    {
        texture_data.set(program_bits[i], cur_offset);
        cur_offset += program_bits[i].length;
    }

    // Add HALT to end of instruction stream.
    texture_data.set([rt_prog_encode_int(0)], cur_offset);

    return texture_data;
}

function rt_prog_setup()
{
    // Load vertex data into a card-side buffer.  We only really use the vertex
    // data to get a "billboard" on which to run our fragment shader, so it
    // never changes.
    vertex_buffer = gl.createBuffer();

    gl.bindBuffer(gl.ARRAY_BUFFER, vertex_buffer);

    var vertex_array = new Float32Array([
        1.0,  1.0,
            -1.0, 1.0,
        1.0,  -1.0,
            -1.0, -1.0,
    ]);

    gl.bufferData(gl.ARRAY_BUFFER, vertex_array, gl.STATIC_DRAW);

    // The vertex shader only takes one attribute.
    rt_prog_vertex_pos = gl.getAttribLocation(rt_prog, "vertex_pos");

    // Extract the uniform inputs to rt_prog.
    rt_prog_use();
    rt_prog_viewport = gl.getUniformLocation(rt_prog, "viewport");
    rt_prog_cs_to_ws_linear = gl.getUniformLocation(rt_prog, "cs_to_ws.l");
    rt_prog_cs_to_ws_offset = gl.getUniformLocation(rt_prog, "cs_to_ws.o");
    rt_prog_cam_aperture = gl.getUniformLocation(rt_prog, "cam_aperture");
    rt_prog_scenedesc = gl.getUniformLocation(rt_prog, "scenedesc");
    rt_prog_scenedesc_dim = gl.getUniformLocation(rt_prog, "scenedesc_dim");
    rt_prog_materialdesc = gl.getUniformLocation(rt_prog, "materialdesc");
    rt_prog_materialdesc_dim = gl.getUniformLocation(rt_prog, "materialdesc_dim");

    // Scene description bytecode.
    var scene_code = new Float32Array([
        rt_prog_encode_int(2), // Set material 1.
        rt_prog_encode_int(1),
        rt_prog_encode_int(1), // Test sphere.
        rt_prog_encode_int(1),
        rt_prog_encode_int(2), // Set material 2.
        rt_prog_encode_int(2),
        rt_prog_encode_int(1), // Test plane.
        rt_prog_encode_int(0),
        rt_prog_encode_int(2), // Set material 0 (final material is applied to infinity).
        rt_prog_encode_int(0),
        rt_prog_encode_int(0)  // Halt.
    ]);

    var scene_code_full = new Float32Array(512*512);
    scene_code_full.set(scene_code);

    // Create the texture that will contain geometry data.
    rt_prog_scenedesc_tex = gl.createTexture();
    gl.bindTexture(gl.TEXTURE_2D, rt_prog_scenedesc_tex);
    gl.texImage2D(gl.TEXTURE_2D, 0, gl.ALPHA, 512, 512, 0, gl.ALPHA, gl.FLOAT, scene_code_full);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
    gl.bindTexture(gl.TEXTURE_2D, null);

    material_code_fragments = [
        rt_prog_floatcode_material_constant([0.8, 0.8, 1.0]),
        rt_prog_floatcode_material_lambert([1.0, 0.0, 1.0], 0.1, [1, 0, 2]),
        rt_prog_floatcode_material_lambert([0.0, 1.0, 0.0], 0.1, [1, 0, 2])
    ];

    material_code = new Float32Array(512*512);
    floatcode_write(material_code, material_code_fragments);

    rt_prog_materialdesc_tex = gl.createTexture();
    gl.bindTexture(gl.TEXTURE_2D, rt_prog_materialdesc_tex);
    gl.texImage2D(gl.TEXTURE_2D, 0, gl.ALPHA, 512, 512, 0, gl.ALPHA, gl.FLOAT, material_code);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
    gl.bindTexture(gl.TEXTURE_2D, null);
}

function rt_prog_use()
{
    gl.useProgram(rt_prog);

    // The vertex shader only takes one attribute.  Switch it on.
    gl.enableVertexAttribArray(rt_prog_vertex_pos);

    // Set the format for the data we feed to the "vertex_pos" attribute.
    gl.vertexAttribPointer(rt_prog_vertex_pos, 2, gl.FLOAT, false, 0, 0);
}

function rt_prog_set_viewport(viewport)
{
    gl.uniform2fv(rt_prog_viewport, viewport);
}

function rt_prog_set_cam_aperture(aperture)
{
    gl.uniform3fv(rt_prog_cam_aperture, aperture);
}

function rt_prog_set_cs_to_ws(linear, offset)
{
    gl.uniformMatrix3fv(rt_prog_cs_to_ws_linear, false, linear);
    gl.uniform3fv(rt_prog_cs_to_ws_offset, offset);
}

// Promise adapter around XMLHttpRequest.
function load_text(url)
{
    // Return a new promise.
    return new Promise(function(resolve, reject) {
        // Do the usual XHR stuff
        var req = new XMLHttpRequest();
        req.open('GET', url);

        req.onload = function() {
            // This is called even on 404 etc
            // so check the status
            if (req.status == 200) {
                // Resolve the promise with the response text
                resolve(req.response);
            }
            else {
                // Otherwise reject with the status text
                // which will hopefully be a meaningful error
                reject(Error(req.statusText));
            }
        };

        // Handle network errors
        req.onerror = function() {
            reject(Error("Network Error"));
        };

        // Make the request
        req.send();
    });
}

function start()
{
    // Asynchronously load shader texts from server and compile them.
    Promise.all(
        [load_text("/webgl-raytracer-assets/raytracer.vert"),
         load_text("/webgl-raytracer-assets/raytracer.frag")]
    ).then(
        function(texts) {
            canvas = document.getElementById("glcanvas");

            // Load WebGL.
            try
            {
                gl = canvas.getContext("webgl");
            }
            catch(e)
            {
                console.log("Failed to load WebGL.");
                return;
            }

            glext_oes_texture_float = gl.getExtension('OES_texture_float');
            if(! glext_oes_texture_float)
            {
                console.log("Error loading OES_texture_float extension.");
                return;
            }

            var rt_prog_vert_obj = compile_shader_object(gl.VERTEX_SHADER, texts[0]);
            if(!rt_prog_vert_obj)
                return;
            var rt_prog_frag_obj = compile_shader_object(gl.FRAGMENT_SHADER, texts[1]);
            if(!rt_prog_frag_obj)
                return;

            rt_prog = link_shader_program([rt_prog_vert_obj, rt_prog_frag_obj]);
            if(!rt_prog)
                return;

            rt_prog_setup();

            gl.clearColor(1.0, 0.0, 0.0, 1.0);
            gl.clear(gl.COLOR_BUFFER_BIT);

            setInterval(draw_scene, 30);
        }
    ).catch(
        function(err) {
            console.log("Error async loading shaders: " + err);
            rt_prog = null;
        }
    );
}

function compile_shader_object(type, text)
{
    var shader_object = gl.createShader(type);
    gl.shaderSource(shader_object, text);
    gl.compileShader(shader_object);

    if(!gl.getShaderParameter(shader_object, gl.COMPILE_STATUS))
    {
        console.log("Error compiling shader: " + gl.getShaderInfoLog(shader_object));
        return null;
    }

    return shader_object;
}

function link_shader_program(shader_objects)
{
    var program = gl.createProgram();
    for(var i in shader_objects)
    {
        gl.attachShader(program, shader_objects[i]);
    }
    gl.linkProgram(program);

    if (!gl.getProgramParameter(program, gl.LINK_STATUS))
    {
        console.log("Error linking shader program: " + gl.getProgramInfoLog(program));
        return null;
    }

    return program;
}

var cur_time = 0.0;
var first = true;
function draw_scene()
{
    gl.clear(gl.COLOR_BUFFER_BIT /*| gl.DEPTH_BUFFER_BIT*/);

    rt_prog_use();

    if(first)
    {
        rt_prog_set_viewport(new Float32Array([canvas.width, canvas.height]));
        rt_prog_set_cam_aperture(new Float32Array([0.02, 0.0192, 0.0108]));
        rt_prog_set_cs_to_ws(
            new Float32Array([
                1, 0, 0,
                0, 1, 0,
                0, 0, 1
            ]),
            new Float32Array([-5, 0, 1])
        );

        gl.uniform1i(rt_prog_scenedesc, 0);
        gl.uniform1i(rt_prog_scenedesc_dim, 512);

        gl.uniform1i(rt_prog_materialdesc, 1);
        gl.uniform1i(rt_prog_materialdesc_dim, 512);

        first = false;
    }

    cur_time += 0.030;
    light_x = Math.cos(cur_time);
    light_y = Math.sin(cur_time);

    material_code_fragments[1] = rt_prog_floatcode_material_lambert([1.0, 0.0, 1.0], 0.1, [light_x, light_y, 2]);
    material_code_fragments[2] = rt_prog_floatcode_material_lambert([0.0, 1.0, 0.0], 0.1, [light_x, light_y, 2]);

    material_code = floatcode_write(material_code, material_code_fragments);

    gl.activeTexture(gl.TEXTURE0);
    gl.bindTexture(gl.TEXTURE_2D, rt_prog_scenedesc_tex);

    gl.activeTexture(gl.TEXTURE1);
    gl.bindTexture(gl.TEXTURE_2D, rt_prog_materialdesc_tex);
    gl.texImage2D(gl.TEXTURE_2D, 0, gl.ALPHA, 512, 512, 0, gl.ALPHA, gl.FLOAT, material_code);

    gl.bindBuffer(gl.ARRAY_BUFFER, vertex_buffer);
    gl.drawArrays(gl.TRIANGLE_STRIP, 0, 4);
}

// Immediate code
start();
