FROM centos:7 as build

ARG version
ARG commit

RUN yum install -y rpm-build make

ENV GOLANG_VERSION 1.13.4
RUN curl -sSL https://dl.google.com/go/go${GOLANG_VERSION}.linux-amd64.tar.gz \
    | tar -C /usr/local -xz
ENV GOPATH /go
ENV PATH $GOPATH/bin:/usr/local/go/bin:$PATH

RUN mkdir -p /root/rpmbuild/{SPECS,SOURCES}

COPY gpu-admission.spec /root/rpmbuild/SPECS
COPY gpu-admission-source.tar.gz /root/rpmbuild/SOURCES

RUN echo '%_topdir /root/rpmbuild' > /root/.rpmmacros \
          && echo '%__os_install_post %{nil}' >> /root/.rpmmacros \
                  && echo '%debug_package %{nil}' >> /root/.rpmmacros
WORKDIR /root/rpmbuild/SPECS
RUN rpmbuild -ba --quiet \
  --define 'version '${version}'' \
  --define 'commit '${commit}'' \
  gpu-admission.spec


FROM centos:7

ARG version
ARG commit

COPY --from=build /root/rpmbuild/RPMS/x86_64/gpu-admission-${version}-${commit}.el7.x86_64.rpm /tmp

RUN rpm -ivh /tmp/gpu-admission-${version}-${commit}.el7.x86_64.rpm

EXPOSE 3456

CMD ["/bin/bash", "-c", "/usr/bin/gpu-admission --address=0.0.0.0:3456 --v=$LOG_LEVEL --logtostderr=true $EXTRA_FLAGS"]
