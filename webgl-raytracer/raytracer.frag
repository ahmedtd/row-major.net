// For the integer modulo operation.
//#extension GL_EXT_gpu_shader4 : enable

// This is not totally portable.  Need to check GL_FRAGMENT_PRECISION_HIGH.
precision highp float;
precision highp int;
precision highp sampler2D;

uniform vec2 viewport;

#define MATERIAL_MODE_SKIP    0
#define MATERIAL_MODE_EXECUTE 1

struct tform
{
    mat3 l;
    vec3 o;
};

tform tapply(tform tl, tform tr)
{
    return tform(
        tl.l * tr.l,
        tl.o + (tl.l * tr.o)
    );
}

vec3 tapply(tform t, vec3 p)
{
    return t.l * p + t.o;
}

// Transform from camera space to world space.
uniform tform cs_to_ws;

// Camera aperture vector, in camera space.
uniform vec3 cam_aperture;

struct ray
{
    vec3 point; // Origin point.
    vec3 slope; // Normalized slope.
};

vec3 eval(ray r, float t)
{
    return r.point + t * r.slope;
}

ray tapply(tform t, ray r)
{
    return ray(tapply(t, r.point), normalize(t.l * r.slope));
}

// Convert screen-space coordinates to camera space coordinates.
//
// In camera space, the camera is looking along the first (x) axis, the second
// (y) axis is the left vector, and the third (z) axis is the up vector.
//
// A point on the screen is converted to a point on the x=1 plane in camera
// space.
vec3 ss_to_cs(vec2 ss)
{
    vec3 cs = vec3(
        1.0,
        -((ss.x / viewport.x) * 2.0 - 1.0),
        ((ss.y / viewport.y) * 2.0 - 1.0)
    );

    return cs;
}

// Get the camera ray from gl_FragCoord.
ray construct_cam_ray(vec2 ss)
{
    ray cs_ray = ray(vec3(0,0,0), ss_to_cs(ss));
    cs_ray.slope = cs_ray.slope * cam_aperture;

    return tapply(cs_to_ws, cs_ray);
}

#define CONTACT_TYPE_MISS 0
#define CONTACT_TYPE_ENTR 1
#define CONTACT_TYPE_EXIT 2
#define CONTACT_TYPE_SKIM 3

struct contact
{
    int type;
    float t;
    vec3 normal;
    vec3 mtl3;
};

contact contact_make_miss()
{
    contact result;
    result.type = CONTACT_TYPE_MISS;
    return result;
}

struct plane
{
    int unused;
};

contact test_plane(plane mdl_plane, ray ms_ray)
{
    vec3 plane_point  = vec3(0,0,-0.01);
    vec3 plane_normal = vec3(0,0,1);

    float progress = dot(ms_ray.slope, plane_normal);

    if(progress == 0.0)
    {
        // We will never strike the plane.
        return contact_make_miss();
    }
    else
    {
        float height = dot(plane_point, plane_normal) - dot(ms_ray.point, plane_normal);
        float t = height / progress;

        if(t < 0.0)
        {
            // Our ray does not hit the plane.
            return contact_make_miss();
        }
        else
        {
            return contact(
                (height < 0.0) ? CONTACT_TYPE_ENTR : CONTACT_TYPE_EXIT,
                t,
                plane_normal,
                eval(ms_ray, t)
            );
        }
    }
}

struct sphere
{
    int unused;
};

contact test_sphere(sphere mdl, ray ms_ray)
{
    // We know that length(ms_ray.slope) == 1, hence term 'a' is 1.
    float b = dot(ms_ray.point, ms_ray.slope);
    float c = dot(ms_ray.point, ms_ray.point) - 1.0;

    // Term c doubles as the height of the ray's point above the sphere.

    float det = b*b - c;

    if(det < 0.0)
    {
        // Miss the sphere.
        return contact_make_miss();
    }
    else if(det == 0.0)
    {
        // Just barely touch the sphere.
        return contact(
            CONTACT_TYPE_SKIM,
            -b,
            normalize(eval(ms_ray, -b)),
            eval(ms_ray, -b)
        );
    }
    else
    {
        float t_min = -b - sqrt(det);
        float t_max = -b + sqrt(det);

        if(0.0 < t_min)
        {
            // Entering the sphere at t_min
            return contact(
                CONTACT_TYPE_ENTR,
                t_min,
                normalize(eval(ms_ray, t_min)),
                eval(ms_ray, t_min)
            );
        }
        else if(t_min < 0.0 && 0.0 < t_max)
        {
            // Exiting the sphere at t_max.
            return contact(
                CONTACT_TYPE_EXIT,
                t_max,
                normalize(eval(ms_ray, t_min)),
                eval(ms_ray, t_min)
            );
        }
        else /* t_max < 0.0 */
        {
            // No hit, sphere is behind us.
            return contact_make_miss();
        }
    }
}

vec4 checkerboard_mtl3(vec3 mtl3)
{
    // float period = 100.0;
    // mtl3 /= period;

    ivec3 parityvec = ivec3(floor(mod(mtl3, 2.0)));
    int parity = parityvec.x + parityvec.y + parityvec.z;

    if(parity == 0 || parity == 2)
        return vec4(1.0, 0.0, 0.0, 1.0);
    else
        return vec4(0.0, 1.0, 0.0, 1.0);
}

uniform sampler2D scenedesc;
uniform int scenedesc_dim;

uniform sampler2D materialdesc;
uniform int materialdesc_dim;

// Won't work if amount is > scenedesc_dim.
ivec2 scenedesc_advance_ip(ivec2 ip, int amount)
{
    ivec2 new_ip = ip;

    new_ip.x += amount;
    if(new_ip.x > scenedesc_dim)
    {
        new_ip.x = new_ip.x - scenedesc_dim;
        ++new_ip.y;
    }

    return new_ip;
}

float scenedesc_decode_raw(ivec2 ip)
{
    float val = texture2D(scenedesc, vec2(ip) / vec2(scenedesc_dim, scenedesc_dim)).a;
    return val;
}

int scenedesc_decode_int(ivec2 ip)
{
    float val = scenedesc_decode_raw(ip);
    return int(val * 65536.0);
}

float scenedesc_decode_float(ivec2 ip)
{
    float val = scenedesc_decode_raw(ip);
    return (val - 1.0/2.0) * 2.0 * 65536.0;
}

ivec2 materialdesc_advance_ip(ivec2 ip, int amount)
{
    ivec2 new_ip = ip;

    new_ip.x += amount;
    if(new_ip.x > materialdesc_dim)
    {
        new_ip.x = new_ip.x - materialdesc_dim;
        ++new_ip.y;
    }

    return new_ip;
}

float materialdesc_decode_raw(ivec2 ip)
{
    float val = texture2D(materialdesc, vec2(ip) / vec2(scenedesc_dim, scenedesc_dim)).a;
    return val;
}

int materialdesc_decode_int(ivec2 ip)
{
    float val = materialdesc_decode_raw(ip);
    return int(val * 65536.0);
}

float materialdesc_decode_float(ivec2 ip)
{
    float val = materialdesc_decode_raw(ip);
    return (val - 1.0/2.0) * 2.0 * 65536.0;
}

contact test_dispatcher(inout ivec2 ip, ray cur_ray)
{
    int geom_type = scenedesc_decode_int(ip);
    ip = scenedesc_advance_ip(ip, 1);

    contact test_result = contact_make_miss();

    if(geom_type == 0)
    {
        // Plane.

        // Nothing to decode.
        plane decode_plane;
        test_result = test_plane(decode_plane, cur_ray);
    }
    else if(geom_type == 1)
    {
        // Sphere.

        sphere decode_sphere;
        test_result = test_sphere(decode_sphere, cur_ray);
    }
    else
    {
        // Unknown geom type.
    }

    return test_result;
}

struct search_scene_result
{
    contact min_contact;
    int material_identifier;
};

search_scene_result scene_interpreter(ray ws_query)
{
    // Minimum state.
    int min_material_identifier = 0;
    contact min_contact = contact_make_miss();
    min_contact.t = 1e20;
    min_contact.normal = -ws_query.slope;
    min_contact.mtl3 = ws_query.slope;

    // State held during execution of the floatcode stream.
    int cur_material_identifier = 0;

    ivec2 ip = ivec2(0,0);
    for(int fake_ip = 0; fake_ip < 32; ++fake_ip)
    {
        int ins = scenedesc_decode_int(ip);
        ip = scenedesc_advance_ip(ip, 1);

        if(ins == 0)
        {
            // Halt, optionally catch fire.
            if(min_contact.type != CONTACT_TYPE_MISS)
            {
                return search_scene_result(min_contact, min_material_identifier);
            }
            else
            {
                // If we didn't hit anything, perform shading with the last
                // active material, at infinity.
                return search_scene_result(min_contact, cur_material_identifier);
            }
        }
        else if(ins == 1)
        {
            // Test geometry.

            contact test_contact = test_dispatcher(ip, ws_query);

            if(CONTACT_TYPE_MISS != test_contact.type
               && test_contact.t < min_contact.t)
            {
                min_contact = test_contact;
                min_material_identifier = cur_material_identifier;
            }
        }
        else if(ins == 2)
        {
            // Set current material.
            cur_material_identifier = scenedesc_decode_int(ip);
            ip = scenedesc_advance_ip(ip, 1);
        }
        else
        {
            // Unknown instruction.
            search_scene_result invalid;
            return invalid;
        }
    }

    return search_scene_result(min_contact, cur_material_identifier);
}

struct shade_result
{
    vec4 color;
};

shade_result shade_result_make_skip()
{
    shade_result empty;
    return empty;
}

shade_result material_decode_constant(inout ivec2 ip, int mode, ray outbound_ray, contact shade_contact)
{
    if(MATERIAL_MODE_SKIP == mode)
    {
        ip = materialdesc_advance_ip(ip, 3);
        return shade_result_make_skip();
    }

    // We have three raw floats to decode.
    vec3 color;
    color.r = materialdesc_decode_raw(ip); ip = materialdesc_advance_ip(ip, 1);
    color.g = materialdesc_decode_raw(ip); ip = materialdesc_advance_ip(ip, 1);
    color.b = materialdesc_decode_raw(ip); ip = materialdesc_advance_ip(ip, 1);

    shade_result result;
    result.color = vec4(color, 1.0);
    return result;
}

struct material_lambert
{
    vec3 color;
    float ambient_illuminance;
    vec3 light_point;
};

shade_result material_decode_lambert(inout ivec2 ip, int mode, ray outbound_ray, contact shade_contact)
{
    if(MATERIAL_MODE_SKIP == mode)
    {
        ip = materialdesc_advance_ip(ip, 7);
        return shade_result_make_skip();
    }

    material_lambert params;

    params.color.r = materialdesc_decode_raw(ip); ip = materialdesc_advance_ip(ip, 1);
    params.color.g = materialdesc_decode_raw(ip); ip = materialdesc_advance_ip(ip, 1);
    params.color.b = materialdesc_decode_raw(ip); ip = materialdesc_advance_ip(ip, 1);

    params.ambient_illuminance = materialdesc_decode_raw(ip); ip = materialdesc_advance_ip(ip, 1);

    params.light_point.x = materialdesc_decode_float(ip); ip = materialdesc_advance_ip(ip, 1);
    params.light_point.y = materialdesc_decode_float(ip); ip = materialdesc_advance_ip(ip, 1);
    params.light_point.z = materialdesc_decode_float(ip); ip = materialdesc_advance_ip(ip, 1);

    vec3 contact_point = eval(outbound_ray, shade_contact.t);
    vec3 light_dir = normalize(params.light_point - contact_point);
    float light_distance = length(params.light_point - contact_point);

    float illuminance = params.ambient_illuminance;

    // Cast to light point.
    search_scene_result shadow_test = scene_interpreter(ray(contact_point+0.001*light_dir, light_dir));
    if(shadow_test.min_contact.type == CONTACT_TYPE_MISS
       || shadow_test.min_contact.t < 0.0
       || light_distance < shadow_test.min_contact.t)
    {
        illuminance = max(illuminance, dot(shade_contact.normal, light_dir));
    }

    shade_result result;
    result.color = vec4(params.color * illuminance, 1.0);
    return result;
}

shade_result material_decode(inout ivec2 ip, int mode, ray reflected_ray, contact shade_contact)
{
    // Decode material type.
    int material_type = materialdesc_decode_int(ip);
    ip = materialdesc_advance_ip(ip, 1);

    if(material_type == 0)
    {
        // material_constant.
        return material_decode_constant(ip, mode, reflected_ray, shade_contact);
    }
    else if(material_type == 1)
    {
        // material_lambert.
        return material_decode_lambert(ip, mode, reflected_ray, shade_contact);
    }
    else
    {
        // Unknown material.
        shade_result none;
        return none;
    }
}

shade_result material_interpreter(int selected_identifier, ray outbound_ray, contact selected_contact)
{
    int cur_identifier = 0;
    ivec2 ip = ivec2(0,0);
    for(int fake_ip = 0; fake_ip < 32; ++fake_ip)
    {
        int ins = materialdesc_decode_int(ip);
        ip = materialdesc_advance_ip(ip, 1);

        if(ins == 0)
        {
            // Halt.
            shade_result invalid;
            invalid.color = vec4(1.0, 0.0, 0.0, 1.0);
            return invalid;
        }
        else if(ins == 1)
        {
            // Define material.

            if(cur_identifier == selected_identifier)
                return material_decode(ip, MATERIAL_MODE_EXECUTE, outbound_ray, selected_contact);
            else
                material_decode(ip, MATERIAL_MODE_SKIP, outbound_ray, selected_contact);

            ++cur_identifier;
        }
        else
        {
            // Unknown instruction.
            shade_result invalid;
            invalid.color = vec4(1.0, 1.0, 0.0, 1.0);
            return invalid;
        }
    }

    shade_result invalid;
    return invalid;
}

void main()
{
    ray root_ray = construct_cam_ray(gl_FragCoord.xy);

    search_scene_result result = scene_interpreter(root_ray);

    shade_result the_shade = material_interpreter(
        result.material_identifier,
        root_ray,
        result.min_contact
    );

    gl_FragColor = the_shade.color;
}
