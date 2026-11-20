//go:build drm && linux && cgo
#include "drm_bridge.h"

#include <errno.h>
#include <fcntl.h>
#include <poll.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mman.h>
#include <unistd.h>

#include <xf86drm.h>
#include <xf86drmMode.h>
#include <drm.h>
#include <drm_mode.h>

typedef struct {
    uint32_t handle, pitch, fb;
    uint64_t size;
    unsigned char *map;
} PixelBuffer;

struct RoundDisplay {
    int fd;
    uint32_t connector_id;
    uint32_t crtc_id;
    drmModeModeInfo mode;
    drmModeCrtcPtr original;
    PixelBuffer buf[2];
    int front, begun;
};

static void seterr(char *out,size_t cap,const char *what) {
    if(out && cap) snprintf(out,cap,"%s: %s",what,strerror(errno));
}

static void release_buffer(RoundDisplay *d,PixelBuffer *b) {
    if(b->map && b->map!=MAP_FAILED) munmap(b->map,(size_t)b->size);
    b->map=NULL;
    if(b->fb) drmModeRmFB(d->fd,b->fb);
    b->fb=0;
    if(b->handle) {
        struct drm_mode_destroy_dumb destroy={0};
        destroy.handle=b->handle;
        drmIoctl(d->fd,DRM_IOCTL_MODE_DESTROY_DUMB,&destroy);
        b->handle=0;
    }
}

void round_close(RoundDisplay *d) {
    if (!d) return;
    if(d->fd>=0) {
        if(d->begun && d->original && d->original->mode_valid) {
            // Best effort: a stopped display manager may have released the old FB.
            (void)drmModeSetCrtc(d->fd,d->original->crtc_id,
                d->original->buffer_id,d->original->x,d->original->y,
                &d->connector_id,1,&d->original->mode);
        }
        for(int i=0;i<2;i++) release_buffer(d,&d->buf[i]);
        close(d->fd);
    }
    if(d->original) drmModeFreeCrtc(d->original);
    free(d);
}

static int make_buffer(RoundDisplay *d,PixelBuffer *b,char *error,size_t cap) {
    struct drm_mode_create_dumb create={0};
    create.width=480; create.height=480;create.bpp=32;
    if(drmIoctl(d->fd,DRM_IOCTL_MODE_CREATE_DUMB,&create)) {
        seterr(error,cap,"DRM create dumb buffer");return -1;
    }
    b->handle=create.handle;b->pitch=create.pitch;b->size=create.size;
    if(drmModeAddFB(d->fd,480,480,24,32,b->pitch,b->handle,&b->fb)) {
        seterr(error,cap,"DRM add framebuffer");return -1;
    }
    struct drm_mode_map_dumb map={0};map.handle=b->handle;
    if(drmIoctl(d->fd,DRM_IOCTL_MODE_MAP_DUMB,&map)) {
        seterr(error,cap,"DRM map dumb buffer");return -1;
    }
    b->map=mmap(NULL,(size_t)b->size,PROT_READ|PROT_WRITE,MAP_SHARED,d->fd,(off_t)map.offset);
    if(b->map==MAP_FAILED) {
        b->map=NULL;seterr(error,cap,"mmap DRM framebuffer");return -1;
    }
    memset(b->map,0,(size_t)b->size);
    return 0;
}

RoundDisplay *round_open(const char *device,const char *connector,char *error,size_t cap) {
    RoundDisplay *d=calloc(1,sizeof(*d));
    if(!d) {seterr(error,cap,"allocate display");return NULL;}
    d->fd=-1;d->front=0;
    d->fd=open(device,O_RDWR|O_CLOEXEC);
    if(d->fd<0) {seterr(error,cap,"open DRM card");goto fail;}
    drmModeResPtr res=drmModeGetResources(d->fd);
    if(!res) {seterr(error,cap,"enumerate DRM resources");goto fail;}
    drmModeConnectorPtr chosen=NULL;
    for(int i=0;i<res->count_connectors;i++) {
        drmModeConnectorPtr c=drmModeGetConnector(d->fd,res->connectors[i]);
        if(!c)continue;
        const char *name=drmModeGetConnectorTypeName(c->connector_type);
        char candidate[128];
        snprintf(candidate,sizeof(candidate),"%s-%u",name?name:"unknown",c->connector_type_id);
        if(!strcmp(candidate,connector) && c->connection==DRM_MODE_CONNECTED) {
            chosen=c;break;
        }
        drmModeFreeConnector(c);
    }
    if(!chosen) {
        if(error&&cap)snprintf(error,cap,"DRM connector %s not found or disconnected",connector);
        drmModeFreeResources(res);goto fail;
    }
    d->connector_id=chosen->connector_id;
    int found_mode=0;
    for(int i=0;i<chosen->count_modes;i++) {
        if(chosen->modes[i].hdisplay==480 && chosen->modes[i].vdisplay==480) {
            d->mode=chosen->modes[i];found_mode=1;break;
        }
    }
    if(!found_mode) {
        if(error&&cap)snprintf(error,cap,"connector %s has no 480x480 mode",connector);
        drmModeFreeConnector(chosen);drmModeFreeResources(res);goto fail;
    }
    drmModeEncoderPtr enc=NULL;
    if(chosen->encoder_id)enc=drmModeGetEncoder(d->fd,chosen->encoder_id);
    if(enc && enc->crtc_id) d->crtc_id=enc->crtc_id;
    if(!d->crtc_id) {
        // Find a compatible CRTC from all encoders, not a guessed index.
        for(int j=0;j<chosen->count_encoders && !d->crtc_id;j++) {
            drmModeEncoderPtr e=drmModeGetEncoder(d->fd,chosen->encoders[j]);
            if(!e)continue;
            for(int k=0;k<res->count_crtcs;k++) {
                if(e->possible_crtcs & (1u<<k)) {d->crtc_id=res->crtcs[k];break;}
            }
            drmModeFreeEncoder(e);
        }
    }
    if(enc)drmModeFreeEncoder(enc);
    drmModeFreeConnector(chosen);drmModeFreeResources(res);
    if(!d->crtc_id) {
        if(error&&cap)snprintf(error,cap,"no CRTC supports %s",connector);
        goto fail;
    }
    d->original=drmModeGetCrtc(d->fd,d->crtc_id);
    if(make_buffer(d,&d->buf[0],error,cap)<0)goto fail;
    if(make_buffer(d,&d->buf[1],error,cap)<0)goto fail;
    return d;
fail:
    round_close(d);
    return NULL;
}

static void flipped(int fd,unsigned int frame,unsigned int sec,unsigned int usec,void *data) {
    (void)fd;(void)frame;(void)sec;(void)usec;
    *(int *)data=1;
}

int round_present(RoundDisplay *d,const unsigned char *rgba,char *error,size_t cap) {
    if(!d||!rgba) {if(error&&cap)snprintf(error,cap,"invalid display or pixel buffer");return -1;}
    int next=d->begun?1-d->front:0;
    PixelBuffer *b=&d->buf[next];
    for(int y=0;y<480;y++) {
        const unsigned char *src=rgba + (size_t)y*480*4;
        unsigned char *dst=b->map + (size_t)y*b->pitch;
        for(int x=0;x<480;x++) {
            dst[x*4+0]=src[x*4+2]; // B
            dst[x*4+1]=src[x*4+1]; // G
            dst[x*4+2]=src[x*4+0]; // R
            dst[x*4+3]=0;         // X in XRGB8888
        }
    }
    if(!d->begun) {
        if(drmModeSetCrtc(d->fd,d->crtc_id,b->fb,0,0,&d->connector_id,1,&d->mode)) {
            seterr(error,cap,"DRM modeset (stop LightDM before testing)");return -1;
        }
        d->front=next;d->begun=1;return 0;
    }
    int done=0;
    if(drmModePageFlip(d->fd,d->crtc_id,b->fb,DRM_MODE_PAGE_FLIP_EVENT,&done)) {
        seterr(error,cap,"DRM page flip");return -1;
    }
    drmEventContext ev={0};ev.version=DRM_EVENT_CONTEXT_VERSION;
    ev.page_flip_handler=flipped;
    while(!done) {
        struct pollfd pfd={.fd=d->fd,.events=POLLIN};
        int n=poll(&pfd,1,1500);
        if(n<=0) {
            if(error&&cap)snprintf(error,cap,"timeout/error waiting for page flip: %s",n==0?"timeout":strerror(errno));
            return -1;
        }
        if(drmHandleEvent(d->fd,&ev)) {
            seterr(error,cap,"DRM handle page flip event");return -1;
        }
    }
    d->front=next;
    return 0;
}
